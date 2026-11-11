package openai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"iter"
	"math"
	"net/http"
	"strings"
	"sync"

	"github.com/coder/websocket"

	"go.chanx.dev/kit/kits/audiostreamin"
)

// Transcriber is OpenAI realtime transcription.
type Transcriber struct {
	// APIKey defaults to OPENAI_API_KEY.
	APIKey string
	URL    string
	Model  string
	// TurnDetection such as {"type": "server_vad"} ends turns on silence, which barge-in
	// needs; it needs gpt-4o-transcribe or -mini, since gpt-live-transcribe refuses it.
	TurnDetection map[string]any
}

// Topic is a transcriber topic on OpenAI.
func (t Transcriber) Topic() audiostreamin.TranscriberTopic {
	return audiostreamin.TranscriberTopic{OpenStream: t.Open, ProviderAvailable: t.Available}
}

func (t Transcriber) Available() bool { return key(t.APIKey) != "" }

func (t Transcriber) SessionUpdate(audio audiostreamin.AudioStart) map[string]any {
	transcription := map[string]any{"model": or(t.Model, "gpt-live-transcribe")}
	if audio.Language != nil && *audio.Language != "" {
		transcription["language"] = *audio.Language
	}
	return map[string]any{
		"type": "session.update",
		"session": map[string]any{
			"type": "transcription",
			"audio": map[string]any{"input": map[string]any{
				"format":         map[string]any{"type": "audio/pcm", "rate": or(audio.SampleRate, PCMRate)},
				"transcription":  transcription,
				"turn_detection": t.TurnDetection,
			}},
		},
	}
}

func (t Transcriber) Open(ctx context.Context, audio audiostreamin.AudioStart) (audiostreamin.TranscriberStream, error) {
	apiKey := key(t.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("%w: OPENAI_API_KEY is not set", audiostreamin.ErrProviderUnavailable)
	}
	if or(audio.SampleRate, PCMRate) != PCMRate || or(audio.Channels, 1) != 1 {
		return nil, fmt.Errorf("%w: OpenAI realtime takes PCM16 mono at 24 kHz", audiostreamin.ErrProviderUnavailable)
	}
	conn, resp, err := websocket.Dial(ctx, or(t.URL, "wss://api.openai.com/v1/realtime?intent=transcription"), &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": {"Bearer " + apiKey}},
	})
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("%w: OpenAI refused the connection (%d)", audiostreamin.ErrProviderUnavailable, resp.StatusCode)
		}
		return nil, fmt.Errorf("%w: OpenAI is unreachable: %v", audiostreamin.ErrProviderUnavailable, err)
	}
	conn.SetReadLimit(-1)
	s := &stream{conn: conn, partials: map[string]string{}}
	if err := s.event(ctx, t.SessionUpdate(audio)); err != nil {
		conn.CloseNow()
		return nil, err
	}
	return s, nil
}

// stream is one realtime transcription session: base64 PCM up, text deltas down.
type stream struct {
	conn *websocket.Conn

	mu        sync.Mutex
	ms        float64
	partials  map[string]string
	finishing bool
	done      bool
}

func (s *stream) Send(ctx context.Context, audio []byte) error {
	err := s.event(ctx, map[string]any{"type": "input_audio_buffer.append", "audio": base64.StdEncoding.EncodeToString(audio)})
	s.mu.Lock()
	s.ms += float64(len(audio)) / (PCMRate * 2) * 1000
	s.mu.Unlock()
	return err
}

// Keepalive does nothing: the session stays open while idle.
func (s *stream) Keepalive(context.Context) error { return nil }

func (s *stream) Finish(ctx context.Context) error {
	s.mu.Lock()
	s.finishing = true
	s.mu.Unlock()
	return s.event(ctx, map[string]any{"type": "input_audio_buffer.commit"})
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
	var e struct {
		Type         string `json:"type"`
		ItemID       string `json:"item_id"`
		AudioStartMs *int   `json:"audio_start_ms"`
		Delta        string `json:"delta"`
		Transcript   string `json:"transcript"`
		Error        struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &e) != nil {
		return nil, false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := int(math.Round(s.ms))
	switch e.Type {
	case "input_audio_buffer.speech_started":
		return []audiostreamin.Frame{audiostreamin.SpeechStartedFrame{AtMs: *or(e.AudioStartMs, &now)}}, false, nil
	case "conversation.item.input_audio_transcription.delta":
		s.partials[e.ItemID] += e.Delta
		if text := strings.TrimSpace(s.partials[e.ItemID]); text != "" {
			return []audiostreamin.Frame{audiostreamin.PartialFrame{Text: text, EndMs: now}}, false, nil
		}
	case "conversation.item.input_audio_transcription.completed":
		delete(s.partials, e.ItemID)
		s.done = s.finishing
		var frames []audiostreamin.Frame
		if text := strings.TrimSpace(e.Transcript); text != "" {
			frames = append(frames, audiostreamin.FinalFrame{Text: text, EndMs: now})
		}
		// Each committed item is one turn.
		return append(frames, audiostreamin.UtteranceEndFrame{}), s.done, nil
	case "error":
		// Committing an empty buffer at the end is not a failure.
		if s.finishing && strings.Contains(e.Error.Code, "buffer") {
			s.done = true
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("OpenAI: %s", e.Error.Message)
	}
	return nil, false, nil
}

func (s *stream) event(ctx context.Context, event map[string]any) error {
	raw, _ := json.Marshal(event)
	if err := s.conn.Write(ctx, websocket.MessageText, raw); err != nil && websocket.CloseStatus(err) == -1 {
		return err
	}
	return nil
}
