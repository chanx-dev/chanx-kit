package elevenlabs

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"iter"
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/coder/websocket"

	"go.chanx.dev/kit/kits/audiostreamin"
)

var scribeErrors = []string{
	"error", "auth_error", "quota_exceeded", "rate_limited", "resource_exhausted",
	"session_time_limit_exceeded", "chunk_size_exceeded", "input_error",
}

// Transcriber is ElevenLabs Scribe realtime speech-to-text.
type Transcriber struct {
	// APIKey defaults to ELEVENLABS_API_KEY.
	APIKey  string
	URL     string
	ModelID string
	Options map[string]string
}

// Topic is a transcriber topic on ElevenLabs.
func (t Transcriber) Topic() audiostreamin.TranscriberTopic {
	return audiostreamin.TranscriberTopic{OpenStream: t.Open, ProviderAvailable: t.Available}
}

func (t Transcriber) Available() bool { return key(t.APIKey) != "" }

func (t Transcriber) RealtimeURL(audio audiostreamin.AudioStart) (string, error) {
	rate := or(audio.SampleRate, audiostreamin.DefaultAudioConfig.SampleRate)
	if !supported(rate, or(audio.Channels, 1)) {
		return "", fmt.Errorf("%w: ElevenLabs takes mono PCM at %v Hz", audiostreamin.ErrProviderUnavailable, PCMRates)
	}
	query := url.Values{
		"model_id":        {or(t.ModelID, "scribe_v2_realtime")},
		"audio_format":    {"pcm_" + strconv.Itoa(rate)},
		"commit_strategy": {"vad"},
	}
	if audio.Language != nil && *audio.Language != "" {
		query.Set("language_code", *audio.Language)
	}
	for k, v := range t.Options {
		query.Set(k, v)
	}
	return or(t.URL, "wss://api.elevenlabs.io/v1/speech-to-text/realtime") + "?" + query.Encode(), nil
}

func (t Transcriber) Open(ctx context.Context, audio audiostreamin.AudioStart) (audiostreamin.TranscriberStream, error) {
	apiKey := key(t.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("%w: ELEVENLABS_API_KEY is not set", audiostreamin.ErrProviderUnavailable)
	}
	address, err := t.RealtimeURL(audio)
	if err != nil {
		return nil, err
	}
	conn, err := dial(ctx, address, apiKey, audiostreamin.ErrProviderUnavailable)
	if err != nil {
		return nil, err
	}
	return &stream{conn: conn, rate: or(audio.SampleRate, audiostreamin.DefaultAudioConfig.SampleRate)}, nil
}

// stream is one Scribe session: base64 PCM up, partial and committed text down.
type stream struct {
	conn *websocket.Conn
	rate int

	mu           sync.Mutex
	ms           float64
	segmentStart int
	finishing    bool
	// Scribe has no speech-started event; an utterance's first partial stands in.
	inUtterance bool
	done        bool
}

func (s *stream) Send(ctx context.Context, audio []byte) error {
	err := s.chunk(ctx, audio, false)
	s.mu.Lock()
	s.ms += float64(len(audio)) / float64(s.rate*2) * 1000
	s.mu.Unlock()
	return err
}

// Keepalive sends an empty chunk: Scribe has no keepalive message.
func (s *stream) Keepalive(ctx context.Context) error { return s.chunk(ctx, nil, false) }

func (s *stream) Finish(ctx context.Context) error {
	s.mu.Lock()
	s.finishing = true
	s.mu.Unlock()
	return s.chunk(ctx, nil, true)
}

func (s *stream) Close() error {
	s.mu.Lock()
	s.finishing = true
	s.mu.Unlock()
	return s.conn.Close(websocket.StatusNormalClosure, "")
}

func (s *stream) Frames(ctx context.Context) iter.Seq2[audiostreamin.Frame, error] {
	return func(yield func(audiostreamin.Frame, error) bool) {
		defer s.conn.Close(websocket.StatusNormalClosure, "")
		for {
			_, raw, err := s.conn.Read(ctx)
			if err != nil {
				s.mu.Lock()
				finishing := s.finishing
				s.mu.Unlock()
				if !finishing && ctx.Err() == nil {
					yield(nil, err)
				}
				return
			}
			frames, done, err := s.framesFor(raw)
			if err != nil {
				yield(nil, err)
				return
			}
			for _, frame := range frames {
				if !yield(frame, nil) {
					return
				}
			}
			if done {
				return
			}
		}
	}
}

func (s *stream) framesFor(raw []byte) ([]audiostreamin.Frame, bool, error) {
	var m struct {
		Type  string `json:"message_type"`
		Text  string `json:"text"`
		Error string `json:"error"`
	}
	if json.Unmarshal(raw, &m) != nil {
		return nil, false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	text := strings.TrimSpace(m.Text)
	now := int(math.Round(s.ms))
	switch {
	case slices.Contains(scribeErrors, m.Type):
		return nil, false, fmt.Errorf("ElevenLabs %s: %s", m.Type, m.Error)
	case m.Type == "partial_transcript" && text != "":
		var frames []audiostreamin.Frame
		if !s.inUtterance {
			s.inUtterance = true
			frames = append(frames, audiostreamin.SpeechStartedFrame{AtMs: s.segmentStart})
		}
		return append(frames, audiostreamin.PartialFrame{Text: text, StartMs: s.segmentStart, EndMs: now}), false, nil
	case m.Type == "committed_transcript":
		var frames []audiostreamin.Frame
		if text != "" {
			frames = append(frames, audiostreamin.FinalFrame{Text: text, StartMs: s.segmentStart, EndMs: now})
		}
		// Scribe commits when voice activity stops.
		s.inUtterance = false
		s.segmentStart = now
		s.done = s.finishing
		return append(frames, audiostreamin.UtteranceEndFrame{}), s.done, nil
	}
	return nil, false, nil
}

func (s *stream) chunk(ctx context.Context, audio []byte, commit bool) error {
	raw, _ := json.Marshal(map[string]any{
		"message_type":  "input_audio_chunk",
		"audio_base_64": base64.StdEncoding.EncodeToString(audio),
		"commit":        commit,
		"sample_rate":   s.rate,
	})
	if err := s.conn.Write(ctx, websocket.MessageText, raw); err != nil && websocket.CloseStatus(err) == -1 {
		return err
	}
	return nil
}
