// Package audiostreamout speaks text to a session's listeners: the synthesizer@1 contract.
package audiostreamout

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"strings"

	chanx "go.chanx.dev/v2"

	"go.chanx.dev/kit/kits/mediastreamout"
)

// Defaults for a SynthesizerTopic without its own stores.
var (
	DefaultReplayStore   mediastreamout.ReplayStore = mediastreamout.NewInMemoryReplayStore()
	DefaultPlaybackStore PlaybackStore              = NewInMemoryPlaybackStore()
	jobs                 mediastreamout.SerialJobs
)

// ErrProviderUnavailable is wrapped by a provider that refuses, e.g. without a key.
var ErrProviderUnavailable = errors.New("the speech provider is unavailable")

// Synthesize yields PCM16 audio for text.
type Synthesize func(ctx context.Context, text string, voice string, format SpeechFormat) iter.Seq2[[]byte, error]

// SynthesizerTopic is speak:<session>: text to speech for everyone on it. Set Synthesize.
// Its Go type is its broadcast group, so two synthesizers need different session ids.
type SynthesizerTopic struct {
	chanx.BaseTopic   `pattern:"speak:{session}" description:"Text to speech for everyone on the session."`
	Synthesize        Synthesize
	ProviderAvailable func() bool
	OutputFormat      *SpeechFormat
	DefaultVoice      string
	// Longer text is refused rather than billed.
	MaxTextChars   int
	MaxChunkMs     int
	ReplayStore    mediastreamout.ReplayStore
	PlaybackStore  PlaybackStore
	OnPlaybackMark func(ctx context.Context, session string, mark PlaybackMark) error
}

func (t SynthesizerTopic) format() SpeechFormat {
	if t.OutputFormat != nil {
		return *t.OutputFormat
	}
	return DefaultSpeechFormat
}

func (t SynthesizerTopic) replay() mediastreamout.ReplayStore {
	if t.ReplayStore != nil {
		return t.ReplayStore
	}
	return DefaultReplayStore
}

func (t SynthesizerTopic) Playback() PlaybackStore {
	if t.PlaybackStore != nil {
		return t.PlaybackStore
	}
	return DefaultPlaybackStore
}

func (t SynthesizerTopic) available() bool {
	return t.ProviderAvailable == nil || t.ProviderAvailable()
}

func key(session string) string { return "SynthesizerTopic:" + session }

func newUtteranceID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (SynthesizerTopic) Passthrough() []chanx.Message {
	return []chanx.Message{SpeechAudioStartMessage{}, SpeechAudioChunkMessage{}, SpeechAudioEndMessage{}, SpeechClearMessage{}, SynthesizerErrorMessage{}}
}

// OnSubscribe replays the utterance in flight: audio cannot be joined part-way.
func (t SynthesizerTopic) OnSubscribe(ctx *chanx.Context) error {
	entries, err := t.replay().Replay(ctx, key(ctx.Params["session"]))
	if err != nil {
		return err
	}
	for _, e := range entries {
		at := *ctx
		at.Seq = &e.Seq
		if err := at.Push(e.Message); err != nil {
			return err
		}
	}
	return nil
}

func (t SynthesizerTopic) HandleSpeak(ctx *chanx.Context, m SpeakMessage) error {
	if !t.available() {
		return ctx.Push(SynthesizerErrorMessage{Payload: SynthesizerError{
			Code: ProviderUnavailable, Message: "The speech provider is not configured.", UtteranceID: m.Payload.UtteranceID,
		}})
	}
	voice, utterance := "", ""
	if m.Payload.Voice != nil {
		voice = *m.Payload.Voice
	}
	if m.Payload.UtteranceID != nil {
		utterance = *m.Payload.UtteranceID
	}
	_, err := t.Speak(ctx, ctx.Publisher(), ctx.Params["session"], m.Payload.Text, voice, utterance)
	return err
}

func (t SynthesizerTopic) HandleSpeakClear(ctx *chanx.Context, _ SpeakClearMessage) error {
	return t.Clear(ctx, ctx.Publisher(), ctx.Params["session"])
}

func (t SynthesizerTopic) HandlePlaybackMark(ctx *chanx.Context, m PlaybackMarkMessage) error {
	session := ctx.Params["session"]
	if err := t.Playback().Mark(ctx, session, m.Payload.UtteranceID, m.Payload.PlayedMs); err != nil {
		return err
	}
	if t.OnPlaybackMark != nil {
		return t.OnPlaybackMark(ctx, session, m.Payload)
	}
	return nil
}

func (SynthesizerTopic) Operations() map[string]chanx.Operation {
	return map[string]chanx.Operation{
		"HandleSpeak": {Summary: "Speak text", Description: "Queue text to be spoken to everyone on the session.",
			Replies: []chanx.Message{SpeechAudioStartMessage{}, SpeechAudioChunkMessage{}, SpeechAudioEndMessage{}, SynthesizerErrorMessage{}}},
		"HandleSpeakClear":   {Summary: "Stop speaking", Description: "Stop the utterance playing and drop everything queued.", Replies: []chanx.Message{SpeechClearMessage{}}},
		"HandlePlaybackMark": {Summary: "Report playback", Description: "How much of an utterance the client has actually played."},
	}
}

// Speak queues text on session and returns its utterance id.
func (t SynthesizerTopic) Speak(ctx context.Context, p *chanx.Publisher, session, text, voice, utteranceID string) (string, error) {
	if utteranceID == "" {
		utteranceID = newUtteranceID()
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return utteranceID, t.emitError(ctx, p, session, BadRequest, "Nothing to say.", utteranceID)
	}
	if limit := orDefault(t.MaxTextChars, 2000); len([]rune(text)) > limit {
		return utteranceID, t.emitError(ctx, p, session, BadRequest, fmt.Sprintf("Text is limited to %d characters.", limit), utteranceID)
	}
	if voice == "" {
		voice = t.DefaultVoice
	}
	jobs.Submit(key(session), func(jobCtx context.Context) error {
		return t.produce(jobCtx, p, session, utteranceID, text, voice)
	})
	return utteranceID, nil
}

// Clear stops the utterance playing and drops the queued ones.
func (t SynthesizerTopic) Clear(ctx context.Context, p *chanx.Publisher, session string) error {
	current := t.current(ctx, session)
	jobs.Cancel(key(session))
	if err := t.replay().Clear(ctx, key(session)); err != nil {
		return err
	}
	return t.broadcast(ctx, p, session, SpeechClearMessage{Payload: SpeechClear{UtteranceID: current}})
}

// IsSpeaking reports whether session has speech playing or queued in this process.
func (SynthesizerTopic) IsSpeaking(session string) bool { return jobs.Busy(key(session)) }

// WaitUntilSpoken blocks until session's queue is sent. For tests.
func (SynthesizerTopic) WaitUntilSpoken(session string) { jobs.Wait(key(session)) }

func orDefault(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func (t SynthesizerTopic) current(ctx context.Context, session string) *string {
	entries, err := t.replay().Replay(ctx, key(session))
	if err != nil || len(entries) == 0 {
		return nil
	}
	switch m := entries[len(entries)-1].Message.(type) {
	case SpeechAudioStartMessage:
		return &m.Payload.UtteranceID
	case SpeechAudioChunkMessage:
		return &m.Payload.UtteranceID
	case SpeechAudioEndMessage:
		return &m.Payload.UtteranceID
	}
	return nil
}

func (t SynthesizerTopic) broadcast(ctx context.Context, p *chanx.Publisher, session string, m chanx.Message, opts ...chanx.BroadcastOption) error {
	return chanx.BroadcastTopic[SynthesizerTopic](ctx, p, "speak:"+session, m, opts...)
}

// emit records m for late joiners, then broadcasts it.
func (t SynthesizerTopic) emit(ctx context.Context, p *chanx.Publisher, session string, m chanx.Message) error {
	seq, err := t.replay().Append(ctx, key(session), m)
	if err != nil {
		return err
	}
	return t.broadcast(ctx, p, session, m, chanx.Seq(seq))
}

func (t SynthesizerTopic) emitError(ctx context.Context, p *chanx.Publisher, session string, code ErrorCode, text, utteranceID string) error {
	return t.broadcast(ctx, p, session, SynthesizerErrorMessage{Payload: SynthesizerError{Code: code, Message: text, UtteranceID: &utteranceID}})
}

func (t SynthesizerTopic) produce(ctx context.Context, p *chanx.Publisher, session, utteranceID, text, voice string) error {
	format := t.format()
	frameBytes := format.Channels * 2
	maxBytes := max(frameBytes, format.SampleRate*orDefault(t.MaxChunkMs, 200)/1000*frameBytes)
	// An emit in progress completes even if the job is cancelled.
	send := context.WithoutCancel(ctx)

	if err := t.replay().Clear(send, key(session)); err != nil {
		return err
	}
	_ = t.Playback().Spoken(send, session, utteranceID, text)
	start := SpeechAudioStart{Encoding: format.Encoding, SampleRate: format.SampleRate, Channels: format.Channels, UtteranceID: utteranceID, Text: text}
	if err := t.emit(send, p, session, SpeechAudioStartMessage{Payload: start}); err != nil {
		return err
	}

	index, total := 0, 0
	var held []byte // a provider chunk can end mid-sample
	var failure error
	if t.Synthesize == nil {
		failure = fmt.Errorf("SynthesizerTopic needs Synthesize")
	} else {
		for piece, err := range t.Synthesize(ctx, text, voice, format) {
			if err != nil {
				failure = err
				break
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			held = append(held, piece...)
			usable := len(held) - len(held)%frameBytes
			for at := 0; at < usable; at += maxBytes {
				data := held[at:min(at+maxBytes, usable)]
				chunk := SpeechAudioChunk{UtteranceID: utteranceID, Index: index, Data: append([]byte(nil), data...)}
				if err := t.emit(send, p, session, SpeechAudioChunkMessage{Payload: chunk}); err != nil {
					return err
				}
				index++
				total += len(data)
			}
			held = append([]byte(nil), held[usable:]...)
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	switch {
	case failure == nil:
	case errors.Is(failure, ErrProviderUnavailable):
		_ = t.emitError(send, p, session, ProviderUnavailable, failure.Error(), utteranceID)
	default:
		slog.Error("synthesis failed", "error", failure)
		_ = t.emitError(send, p, session, ProviderFailed, failure.Error(), utteranceID)
	}

	durationMs := total * 1000 / (format.SampleRate * frameBytes)
	_ = t.Playback().Finished(send, session, utteranceID, durationMs)
	err := t.emit(send, p, session, SpeechAudioEndMessage{Payload: SpeechAudioEnd{UtteranceID: utteranceID, DurationMs: durationMs}})
	_ = t.replay().Clear(send, key(session))
	return err
}
