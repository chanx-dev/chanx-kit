// Package fakevoice transcribes and speaks without a provider, for demos, UI work and CI.
package fakevoice

import (
	"context"
	"encoding/binary"
	"iter"
	"math"
	"strings"
	"sync"

	"go.chanx.dev/kit/kits/audiostreamin"
)

// Loudness is the RMS of PCM16 audio, sampling every 4th sample.
func Loudness(audio []byte) float64 {
	n := len(audio) / 2
	if n == 0 {
		return 0
	}
	var sum float64
	var count int
	for i := 0; i < n; i += 4 {
		v := float64(int16(binary.LittleEndian.Uint16(audio[i*2:])))
		sum += v * v
		count++
	}
	return math.Sqrt(sum / float64(count))
}

// Transcriber hears one scripted word per MsPerWord of loud audio; a pause ends the utterance.
type Transcriber struct {
	Script        []string
	MsPerWord     int
	WordsPerFinal int
	// SpeechLevel is speech RMS in PCM16 units; SilenceMs is the pause that ends an utterance.
	SpeechLevel float64
	SilenceMs   int
}

// DefaultTranscriber says "hello world this is a scripted transcript".
var DefaultTranscriber = Transcriber{
	Script:        strings.Fields("hello world this is a scripted transcript"),
	MsPerWord:     400,
	WordsPerFinal: 4,
	SpeechLevel:   500,
	SilenceMs:     600,
}

// Topic is a transcriber topic using this script. It costs nothing, so nothing is limited.
func (tr Transcriber) Topic() audiostreamin.TranscriberTopic {
	return audiostreamin.TranscriberTopic{
		OpenStream:         tr.Open,
		MaxSession:         audiostreamin.Off,
		IdleTimeout:        audiostreamin.Off,
		MaxSessionsPerUser: audiostreamin.Off,
	}
}

func (tr Transcriber) Open(_ context.Context, audio audiostreamin.AudioStart) (audiostreamin.TranscriberStream, error) {
	rate, channels := audio.SampleRate, audio.Channels
	if rate == 0 {
		rate = audiostreamin.DefaultAudioConfig.SampleRate
	}
	if channels == 0 {
		channels = 1
	}
	return &stream{
		Transcriber: tr,
		bytesPerMs:  float64(rate*channels*2) / 1000,
		frames:      make(chan audiostreamin.Frame, 256),
	}, nil
}

type stream struct {
	Transcriber
	bytesPerMs float64

	mu           sync.Mutex
	frames       chan audiostreamin.Frame
	now          float64
	next         int
	active       bool
	loudMs       float64
	quietMs      float64
	wordsHeard   int
	segment      []string
	segmentStart int
	ended        bool
}

func (s *stream) Send(_ context.Context, audio []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	duration := float64(len(audio)) / s.bytesPerMs
	loud := Loudness(audio) >= s.SpeechLevel
	if loud && !s.active {
		s.active = true
		s.loudMs, s.quietMs, s.wordsHeard = 0, 0, 0
		s.segmentStart = int(math.Round(s.now))
		s.put(audiostreamin.SpeechStartedFrame{AtMs: s.segmentStart})
	}
	s.now += duration
	if !s.active {
		return nil
	}
	if loud {
		s.quietMs = 0
		s.loudMs += duration
		for s.wordsHeard < int(s.loudMs)/s.MsPerWord {
			s.word()
		}
	} else {
		s.quietMs += duration
		if s.wordsHeard > 0 && s.quietMs >= float64(s.SilenceMs) {
			s.endUtterance()
		}
	}
	return nil
}

func (s *stream) Keepalive(context.Context) error { return nil }

func (s *stream) Finish(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active && s.wordsHeard > 0 {
		s.endUtterance()
	}
	s.stop()
	return nil
}

func (s *stream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stop()
	return nil
}

func (s *stream) Frames(ctx context.Context) iter.Seq2[audiostreamin.Frame, error] {
	return func(yield func(audiostreamin.Frame, error) bool) {
		for {
			select {
			case <-ctx.Done():
				return
			case frame, ok := <-s.frames:
				if !ok || !yield(frame, nil) {
					return
				}
			}
		}
	}
}

func (s *stream) word() {
	s.segment = append(s.segment, s.Script[s.next%len(s.Script)])
	s.next++
	s.wordsHeard++
	if len(s.segment) >= s.WordsPerFinal {
		s.final()
		return
	}
	s.put(audiostreamin.PartialFrame{Text: strings.Join(s.segment, " "), StartMs: s.segmentStart, EndMs: int(math.Round(s.now))})
}

func (s *stream) endUtterance() {
	if len(s.segment) > 0 {
		s.final()
	}
	s.put(audiostreamin.UtteranceEndFrame{})
	s.active = false
}

func (s *stream) final() {
	confidence := 1.0
	s.put(audiostreamin.FinalFrame{Text: strings.Join(s.segment, " "), StartMs: s.segmentStart, EndMs: int(math.Round(s.now)), Confidence: &confidence})
	s.segment = nil
	s.segmentStart = int(math.Round(s.now))
}

func (s *stream) put(frame audiostreamin.Frame) {
	if !s.ended {
		s.frames <- frame
	}
}

func (s *stream) stop() {
	if !s.ended {
		s.ended = true
		close(s.frames)
	}
}
