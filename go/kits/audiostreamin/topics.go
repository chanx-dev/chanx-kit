// Package audiostreamin carries audio in and transcripts out: the transcriber@1 contract.
package audiostreamin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	chanx "go.chanx.dev/v2"

	"go.chanx.dev/kit/kits/mediastreamin"
)

// Off turns a limit off.
const Off = -1

// TranscriptTopic is transcript:<session>, a read-only view of a session's transcripts.
type TranscriptTopic struct {
	chanx.BaseTopic `pattern:"transcript:{session}" description:"Watch a session's transcripts read-only."`
}

func (TranscriptTopic) Passthrough() []chanx.Message {
	return []chanx.Message{SpeechStartedMessage{}, TranscriptPartialMessage{}, TranscriptFinalMessage{}, UtteranceEndMessage{}}
}

// Scope is the session a hook is called for.
type Scope struct {
	Session string
	Conn    *chanx.Conn
	// Publisher reaches other connections, such as an agent topic's watchers.
	Publisher *chanx.Publisher
}

// TranscriberTopic is transcribe:<session>: audio in, transcripts out. Set OpenStream.
// Zero limits take the defaults; Off turns one off. Its frames are handled in order.
type TranscriberTopic struct {
	chanx.BaseTopic `pattern:"transcribe:{session}" description:"Audio in, transcripts out." ordered:"true"`
	// OpenStream connects a provider; wrap ErrProviderUnavailable when it refuses.
	OpenStream func(ctx context.Context, audio AudioStart) (TranscriberStream, error)
	// ProviderAvailable is false when the provider cannot work, e.g. without a key.
	ProviderAvailable func() bool
	AudioConfig       *AudioConfig
	// UserID is who stream limits count against; "" counts per connection.
	UserID func(conn *chanx.Conn) string

	// Providers bill per second, so streams are bounded.
	MaxSession         time.Duration
	IdleTimeout        time.Duration
	MaxSessionsPerUser int
	KeepaliveEvery     time.Duration
	FinishTimeout      time.Duration
	QueueChunks        int
	ReorderWindow      int

	// Hooks run after the event is sent.
	OnSpeechStarted func(ctx context.Context, s Scope, e SpeechStarted) error
	OnFinal         func(ctx context.Context, s Scope, t TranscriptFinal) error
	OnUtteranceEnd  func(ctx context.Context, s Scope, e UtteranceEnd) error
}

func pick[T int | time.Duration](value, fallback T) T {
	switch value {
	case Off:
		return 0
	case 0:
		return fallback
	}
	return value
}

func (t TranscriberTopic) config() AudioConfig {
	if t.AudioConfig != nil {
		return *t.AudioConfig
	}
	return DefaultAudioConfig
}

func (t TranscriberTopic) available() bool {
	return t.ProviderAvailable == nil || t.ProviderAvailable()
}

// UserIDOf is a string user, or one with UserID; "" otherwise.
func UserIDOf(conn *chanx.Conn) string {
	switch user := conn.User.(type) {
	case string:
		return user
	case interface{ UserID() string }:
		return user.UserID()
	}
	return ""
}

// session is one provider stream on one subscription.
type session struct {
	mu           sync.Mutex
	scope        Scope
	push         chanx.Context
	stream       TranscriberStream
	sequencer    *mediastreamin.ChunkSequencer
	intake       *mediastreamin.MediaIntake
	held         [][]byte
	endRequested bool
	closed       bool
	finished     bool
	utterance    string
	counted      string
	cancel       context.CancelFunc
	listened     chan struct{}
}

var (
	sessionsMu sync.Mutex
	// Per subscription, and open streams per user; this process only.
	sessions = map[string]*session{}
	open     = map[string]int{}
)

func subscription(ctx *chanx.Context) string { return ctx.Conn.ChannelName + "\n" + ctx.Topic }

func (t TranscriberTopic) OnSubscribe(ctx *chanx.Context) error {
	if err := ctx.Push(AudioConfigMessage{Payload: t.config()}); err != nil {
		return err
	}
	if !t.available() {
		return ctx.Push(errorMessage(ProviderUnavailable, "The transcription provider is not configured."))
	}
	return nil
}

func (t TranscriberTopic) OnUnsubscribe(ctx *chanx.Context) error {
	sessionsMu.Lock()
	s := sessions[subscription(ctx)]
	sessionsMu.Unlock()
	if s != nil {
		t.close(s, false)
	}
	return nil
}

func (t TranscriberTopic) HandleAudioConfigRequest(_ *chanx.Context, _ AudioConfigRequestMessage) (AudioConfigMessage, error) {
	return AudioConfigMessage{Payload: t.config()}, nil
}

func (t TranscriberTopic) HandleAudioStart(ctx *chanx.Context, m AudioStartMessage) error {
	s := &session{
		scope:     Scope{Session: ctx.Params["session"], Conn: ctx.Conn, Publisher: ctx.Publisher()},
		push:      *ctx,
		sequencer: mediastreamin.NewChunkSequencer(pick(t.ReorderWindow, 8)),
		listened:  make(chan struct{}),
	}
	key := subscription(ctx)
	sessionsMu.Lock()
	previous := sessions[key]
	sessions[key] = s
	sessionsMu.Unlock()
	if previous != nil {
		t.close(previous, false)
	}

	if code, text := t.refuse(m.Payload); code != "" {
		return t.abandon(s, key, code, text)
	}
	if !t.claim(s) {
		return t.abandon(s, key, LimitReached, fmt.Sprintf("At most %d streams at once.", pick(t.MaxSessionsPerUser, 2)))
	}
	// Connect off the handler so the route keeps reading; chunks wait in held.
	go t.run(ctx.Conn.Context(), s, key, m.Payload)
	return nil
}

func (t TranscriberTopic) run(connCtx context.Context, s *session, key string, audio AudioStart) {
	stream, err := t.OpenStream(connCtx, audio)
	if err != nil {
		if !errors.Is(err, ErrProviderUnavailable) {
			slog.Error("opening the transcriber stream failed", "error", err)
		}
		t.release(s)
		_ = t.abandon(s, key, ProviderUnavailable, err.Error())
		return
	}

	s.mu.Lock()
	if s.closed { // replaced or left while the provider was connecting
		s.mu.Unlock()
		_ = stream.Close()
		return
	}
	sctx, cancel := context.WithCancel(connCtx)
	s.stream, s.cancel = stream, cancel
	s.intake = &mediastreamin.MediaIntake{
		Sink:           stream.Send,
		OnEnd:          stream.Finish,
		OnQuiet:        func(qctx context.Context, q mediastreamin.Quiet) error { return t.onQuiet(qctx, s, q) },
		MaxChunks:      pick(t.QueueChunks, 50),
		KeepaliveAfter: pick(t.KeepaliveEvery, 4*time.Second),
		IdleAfter:      pick(t.IdleTimeout, 15*time.Second),
	}
	go t.listen(sctx, s)
	s.intake.Start(sctx)
	for _, chunk := range s.held {
		s.intake.Feed(chunk)
	}
	s.held = nil
	ending := s.endRequested
	s.mu.Unlock()

	if limit := pick(t.MaxSession, 300*time.Second); limit > 0 {
		go t.expire(sctx, s, limit)
	}
	if ending {
		t.end(s)
	}
}

func (t TranscriberTopic) refuse(audio AudioStart) (ErrorCode, string) {
	want := t.config()
	encoding := audio.Encoding
	if encoding == "" {
		encoding = PCM16
	}
	rate, channels := audio.SampleRate, audio.Channels
	if rate == 0 {
		rate = DefaultAudioConfig.SampleRate
	}
	if channels == 0 {
		channels = DefaultAudioConfig.Channels
	}
	if encoding != want.Encoding || rate != want.SampleRate || channels != want.Channels {
		return BadFormat, fmt.Sprintf("Expected %s at %d Hz, %d channel(s).", want.Encoding, want.SampleRate, want.Channels)
	}
	if !t.available() {
		return ProviderUnavailable, "The transcription provider is not configured."
	}
	return "", ""
}

func (t TranscriberTopic) abandon(s *session, key string, code ErrorCode, text string) error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	sessionsMu.Lock()
	if sessions[key] == s {
		delete(sessions, key)
	}
	sessionsMu.Unlock()
	return s.push.Push(errorMessage(code, text))
}

func (t TranscriberTopic) HandleAudioChunk(ctx *chanx.Context, m AudioChunkMessage) error {
	sessionsMu.Lock()
	s := sessions[subscription(ctx)]
	sessionsMu.Unlock()
	if s == nil {
		return ctx.Push(errorMessage(NotStarted, "Send audio_start before audio."))
	}
	s.mu.Lock()
	if s.closed || s.endRequested {
		s.mu.Unlock()
		return ctx.Push(errorMessage(NotStarted, "Send audio_start before audio."))
	}
	ready, gaps := s.sequencer.Push(m.Payload.Index, m.Payload.Data)
	s.feed(ready)
	s.mu.Unlock()
	return t.reportGaps(s, gaps)
}

func (t TranscriberTopic) HandleAudioEnd(ctx *chanx.Context, _ AudioEndMessage) error {
	sessionsMu.Lock()
	s := sessions[subscription(ctx)]
	sessionsMu.Unlock()
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.closed || s.endRequested {
		s.mu.Unlock()
		return nil
	}
	s.endRequested = true
	ready, gaps := s.sequencer.Flush()
	s.feed(ready)
	started := s.intake != nil
	s.mu.Unlock()
	if started {
		t.end(s)
	}
	return t.reportGaps(s, gaps)
}

// feed holds chunks until the provider stream opens. Call with s.mu held.
func (s *session) feed(chunks [][]byte) {
	for _, chunk := range chunks {
		if s.intake == nil {
			s.held = append(s.held, chunk)
		} else {
			s.intake.Feed(chunk)
		}
	}
}

// reportGaps names lost chunks rather than feed the provider made-up silence.
func (t TranscriberTopic) reportGaps(s *session, gaps []mediastreamin.Gap) error {
	for _, gap := range gaps {
		if err := s.push.Push(errorMessage(AudioGap, fmt.Sprintf("Chunks %d to %d were lost.", gap.Start, gap.End-1))); err != nil {
			return err
		}
	}
	return nil
}

func (t TranscriberTopic) end(s *session) {
	s.intake.End()
	go func() {
		select {
		case <-s.listened:
			t.close(s, true)
		case <-time.After(pick(t.FinishTimeout, 10*time.Second)):
			t.close(s, false)
		}
	}()
}

func (t TranscriberTopic) onQuiet(ctx context.Context, s *session, quiet mediastreamin.Quiet) error {
	if quiet == mediastreamin.Keepalive {
		return s.stream.Keepalive(ctx)
	}
	t.send(s, errorMessage(LimitReached, "No audio arrived for too long."))
	go t.close(s, false) // close waits for the intake, which is returning
	return nil
}

func (t TranscriberTopic) expire(ctx context.Context, s *session, limit time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(limit):
		t.send(s, errorMessage(LimitReached, fmt.Sprintf("Streams are limited to %g seconds.", limit.Seconds())))
		t.close(s, false)
	}
}

func (t TranscriberTopic) close(s *session, graceful bool) {
	s.mu.Lock()
	if s.finished {
		s.mu.Unlock()
		return
	}
	s.finished, s.closed = true, true
	intake, stream, cancel := s.intake, s.stream, s.cancel
	s.mu.Unlock()
	if intake != nil {
		intake.Stop()
	}
	if !graceful && stream != nil {
		_ = stream.Close()
	}
	if cancel != nil {
		cancel()
	}
	sessionsMu.Lock()
	for key, current := range sessions {
		if current == s {
			delete(sessions, key)
		}
	}
	sessionsMu.Unlock()
	t.release(s)
}

func (t TranscriberTopic) identity(conn *chanx.Conn) string {
	userID := t.UserID
	if userID == nil {
		userID = UserIDOf
	}
	if id := userID(conn); id != "" {
		return id
	}
	return "connection:" + conn.ChannelName
}

func (t TranscriberTopic) claim(s *session) bool {
	identity := t.identity(s.scope.Conn)
	limit := pick(t.MaxSessionsPerUser, 2)
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	if limit > 0 && open[identity] >= limit {
		return false
	}
	open[identity]++
	s.counted = identity
	return true
}

func (t TranscriberTopic) release(s *session) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	identity := s.counted
	if identity == "" {
		return
	}
	s.counted = ""
	if open[identity]--; open[identity] <= 0 {
		delete(open, identity)
	}
}

func (t TranscriberTopic) listen(ctx context.Context, s *session) {
	defer close(s.listened)
	s.mu.Lock()
	stream := s.stream
	s.mu.Unlock()
	if stream == nil {
		return
	}
	for frame, err := range stream.Frames(ctx) {
		s.mu.Lock()
		closed := s.closed
		s.mu.Unlock()
		if err != nil {
			// A stream being closed fails its reads; that is not the provider's fault.
			if ctx.Err() == nil && !closed {
				slog.Error("transcriber stream failed", "error", err)
				t.send(s, errorMessage(ProviderUnavailable, err.Error()))
				go t.close(s, false)
			}
			return
		}
		if closed {
			return
		}
		t.onFrame(ctx, s, frame)
	}
	// The provider closed the stream: after audio_end, or on its own.
	if utterance := s.takeUtterance(); utterance != "" {
		t.utteranceEnd(ctx, s, utterance)
	}
	go t.close(s, true)
}

func (t TranscriberTopic) onFrame(ctx context.Context, s *session, frame Frame) {
	switch f := frame.(type) {
	case SpeechStartedFrame:
		e := SpeechStarted{AtMs: f.AtMs}
		t.publish(ctx, s, SpeechStartedMessage{Payload: e})
		runHook(ctx, s, t.OnSpeechStarted, e)
	case PartialFrame:
		t.publish(ctx, s, TranscriptPartialMessage{Payload: Transcript{
			UtteranceID: s.currentUtterance(), Text: f.Text, StartMs: f.StartMs, EndMs: f.EndMs,
		}})
	case FinalFrame:
		final := TranscriptFinal{
			UtteranceID: s.currentUtterance(), Text: f.Text, StartMs: f.StartMs, EndMs: f.EndMs, Confidence: f.Confidence,
		}
		t.publish(ctx, s, TranscriptFinalMessage{Payload: final})
		runHook(ctx, s, t.OnFinal, final)
	case UtteranceEndFrame:
		if utterance := s.takeUtterance(); utterance != "" {
			t.utteranceEnd(ctx, s, utterance)
		}
	}
}

func (t TranscriberTopic) utteranceEnd(ctx context.Context, s *session, utterance string) {
	e := UtteranceEnd{UtteranceID: utterance}
	t.publish(ctx, s, UtteranceEndMessage{Payload: e})
	runHook(ctx, s, t.OnUtteranceEnd, e)
}

// runHook calls a hook if set; a failing hook is logged, not sent.
func runHook[E any](ctx context.Context, s *session, hook func(context.Context, Scope, E) error, event E) {
	if hook == nil {
		return
	}
	if err := hook(ctx, s.scope, event); err != nil {
		slog.Error("transcriber hook failed", "error", err)
	}
}

func (s *session) currentUtterance() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.utterance == "" {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		s.utterance = hex.EncodeToString(b)
	}
	return s.utterance
}

func (s *session) takeUtterance() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	utterance := s.utterance
	s.utterance = ""
	return utterance
}

// publish reaches this connection and the session's transcript watchers.
func (t TranscriberTopic) publish(ctx context.Context, s *session, m chanx.Message) {
	t.send(s, m)
	if err := chanx.BroadcastTopic[TranscriptTopic](ctx, s.scope.Publisher, "transcript:"+s.scope.Session, m); err != nil {
		slog.Debug("could not reach transcript watchers", "error", err)
	}
}

// send ignores a closed connection: frames come from goroutines that outlive it.
func (t TranscriberTopic) send(s *session, m chanx.Message) {
	if err := s.push.Push(m); err != nil {
		slog.Debug("could not send; the connection is gone", "error", err)
	}
}

func errorMessage(code ErrorCode, text string) TranscriberErrorMessage {
	return TranscriberErrorMessage{Payload: TranscriberError{Code: code, Message: text}}
}

func (TranscriberTopic) Operations() map[string]chanx.Operation {
	events := []chanx.Message{SpeechStartedMessage{}, TranscriptPartialMessage{}, TranscriptFinalMessage{}, UtteranceEndMessage{}, TranscriberErrorMessage{}}
	errs := []chanx.Message{TranscriberErrorMessage{}}
	return map[string]chanx.Operation{
		"HandleAudioConfigRequest": {Summary: "Ask for the capture format", Description: "Return the format to capture audio in, without resubscribing."},
		"HandleAudioStart":         {Summary: "Start streaming audio", Description: "Open a provider stream for audio in the negotiated format.", Replies: events},
		"HandleAudioChunk":         {Summary: "Send audio", Description: "One chunk of the stream, numbered from 0 per audio_start.", Replies: errs},
		"HandleAudioEnd":           {Summary: "End the audio", Description: "Flush the provider, deliver its last transcripts, then close.", Replies: errs},
	}
}
