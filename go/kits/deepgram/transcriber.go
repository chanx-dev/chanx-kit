package deepgram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"math"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/coder/websocket"

	"go.chanx.dev/kit/kits/audiostreamin"
)

type message struct {
	Type        string  `json:"type"`
	Timestamp   float64 `json:"timestamp"`
	IsFinal     bool    `json:"is_final"`
	SpeechFinal bool    `json:"speech_final"`
	Start       float64 `json:"start"`
	Duration    float64 `json:"duration"`
	Channel     struct {
		Alternatives []struct {
			Transcript string   `json:"transcript"`
			Confidence *float64 `json:"confidence"`
		} `json:"alternatives"`
	} `json:"channel"`
}

func ms(seconds float64) int { return int(math.Round(seconds * 1000)) }

// FramesFrom maps one Deepgram server message to transcriber frames.
func FramesFrom(raw []byte) []audiostreamin.Frame {
	var m message
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	switch m.Type {
	case "SpeechStarted":
		return []audiostreamin.Frame{audiostreamin.SpeechStartedFrame{AtMs: ms(m.Timestamp)}}
	case "UtteranceEnd":
		return []audiostreamin.Frame{audiostreamin.UtteranceEndFrame{}}
	case "Results":
		return results(m)
	}
	return nil
}

func results(m message) []audiostreamin.Frame {
	var text string
	var confidence *float64
	if len(m.Channel.Alternatives) > 0 {
		text = strings.TrimSpace(m.Channel.Alternatives[0].Transcript)
		confidence = m.Channel.Alternatives[0].Confidence
	}
	start := ms(m.Start)
	end := start + ms(m.Duration)
	var frames []audiostreamin.Frame
	switch {
	case text == "": // Deepgram sends empty results for silence
	case m.IsFinal:
		frames = append(frames, audiostreamin.FinalFrame{Text: text, StartMs: start, EndMs: end, Confidence: confidence})
	default:
		frames = append(frames, audiostreamin.PartialFrame{Text: text, StartMs: start, EndMs: end})
	}
	// Endpointing: the speaker paused.
	if m.IsFinal && m.SpeechFinal {
		frames = append(frames, audiostreamin.UtteranceEndFrame{})
	}
	return frames
}

// Transcriber is Deepgram live speech-to-text.
type Transcriber struct {
	// APIKey defaults to DEEPGRAM_API_KEY.
	APIKey         string
	URL            string
	Model          string
	Options        map[string]string
	UtteranceEndMs int
	EndpointingMs  int
}

// Topic is a transcriber topic on Deepgram.
func (t Transcriber) Topic() audiostreamin.TranscriberTopic {
	return audiostreamin.TranscriberTopic{OpenStream: t.Open, ProviderAvailable: t.Available}
}

func (t Transcriber) Available() bool { return key(t.APIKey) != "" }

func (t Transcriber) ListenURL(audio audiostreamin.AudioStart) string {
	query := url.Values{
		"model":            {or(t.Model, "nova-3")},
		"encoding":         {"linear16"},
		"sample_rate":      {strconv.Itoa(or(audio.SampleRate, audiostreamin.DefaultAudioConfig.SampleRate))},
		"channels":         {strconv.Itoa(or(audio.Channels, 1))},
		"interim_results":  {"true"},
		"vad_events":       {"true"},
		"utterance_end_ms": {strconv.Itoa(or(t.UtteranceEndMs, 1000))},
		"endpointing":      {strconv.Itoa(or(t.EndpointingMs, 300))},
	}
	if audio.Language != nil && *audio.Language != "" {
		query.Set("language", *audio.Language)
	}
	for k, v := range t.Options {
		query.Set(k, v)
	}
	return or(t.URL, "wss://api.deepgram.com/v1/listen") + "?" + query.Encode()
}

func (t Transcriber) Open(ctx context.Context, audio audiostreamin.AudioStart) (audiostreamin.TranscriberStream, error) {
	apiKey := key(t.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("%w: DEEPGRAM_API_KEY is not set", audiostreamin.ErrProviderUnavailable)
	}
	conn, err := dial(ctx, t.ListenURL(audio), apiKey, audiostreamin.ErrProviderUnavailable)
	if err != nil {
		return nil, err
	}
	return &stream{conn: conn}, nil
}

// stream is one Deepgram live connection: binary audio up, JSON results down.
type stream struct {
	conn    *websocket.Conn
	closing atomic.Bool
}

func (s *stream) Send(ctx context.Context, audio []byte) error {
	return s.conn.Write(ctx, websocket.MessageBinary, audio)
}

func (s *stream) Keepalive(ctx context.Context) error { return s.control(ctx, "KeepAlive") }

// Finish flushes; CloseStream sends the rest and closes, which ends Frames.
func (s *stream) Finish(ctx context.Context) error {
	if err := s.control(ctx, "Finalize"); err != nil {
		return err
	}
	return s.control(ctx, "CloseStream")
}

func (s *stream) Close() error {
	s.closing.Store(true)
	_ = s.control(context.Background(), "CloseStream")
	return s.conn.Close(websocket.StatusNormalClosure, "")
}

func (s *stream) Frames(ctx context.Context) iter.Seq2[audiostreamin.Frame, error] {
	return func(yield func(audiostreamin.Frame, error) bool) {
		for {
			kind, raw, err := s.conn.Read(ctx)
			if err != nil {
				if !s.closing.Load() && !closedNormally(err) && ctx.Err() == nil {
					yield(nil, err)
				}
				return
			}
			if kind != websocket.MessageText {
				continue
			}
			for _, frame := range FramesFrom(raw) {
				if !yield(frame, nil) {
					return
				}
			}
		}
	}
}

func (s *stream) control(ctx context.Context, kind string) error {
	err := s.conn.Write(ctx, websocket.MessageText, fmt.Appendf(nil, `{"type":%q}`, kind))
	if err != nil && (s.closing.Load() || closedNormally(err) || errors.Is(err, net.ErrClosed)) {
		return nil
	}
	return err
}
