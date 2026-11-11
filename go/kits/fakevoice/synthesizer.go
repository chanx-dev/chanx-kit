package fakevoice

import (
	"context"
	"encoding/binary"
	"hash/crc32"
	"iter"
	"math"
	"strings"
	"time"

	"go.chanx.dev/kit/kits/audiostreamout"
)

// WordTones is a tone per word, pitched by the word, then a gap: speech-like pacing.
func WordTones(text string, format audiostreamout.SpeechFormat, wordMs, gapMs int) [][]byte {
	rate := format.SampleRate
	var pieces [][]byte
	for _, word := range strings.Fields(text) {
		pitch := 180 + float64(crc32.ChecksumIEEE([]byte(strings.ToLower(word)))%220)
		tone := rate * wordMs / 1000
		fade := max(1, tone/10)
		gap := rate * gapMs / 1000
		piece := make([]byte, 0, (tone+gap)*format.Channels*2)
		for i := range tone {
			envelope := min(1, float64(i)/float64(fade), float64(tone-i)/float64(fade))
			value := int16(6000 * envelope * math.Sin(2*math.Pi*pitch*float64(i)/float64(rate)))
			for range format.Channels {
				piece = binary.LittleEndian.AppendUint16(piece, uint16(value))
			}
		}
		piece = append(piece, make([]byte, gap*format.Channels*2)...)
		pieces = append(pieces, piece)
	}
	return pieces
}

// Synthesizer speaks any text as a tone per word.
type Synthesizer struct {
	WordMs, GapMs int
	// Pace of 1 streams in real time; 0 sends at once.
	Pace float64
}

var DefaultSynthesizer = Synthesizer{WordMs: 180, GapMs: 70, Pace: 1}

// Topic is a synthesizer topic speaking in tones.
func (sy Synthesizer) Topic() audiostreamout.SynthesizerTopic {
	return audiostreamout.SynthesizerTopic{Synthesize: sy.Synthesize}
}

func (sy Synthesizer) Synthesize(ctx context.Context, text, _ string, format audiostreamout.SpeechFormat) iter.Seq2[[]byte, error] {
	return func(yield func([]byte, error) bool) {
		pause := time.Duration(float64(sy.WordMs+sy.GapMs)*sy.Pace) * time.Millisecond
		for _, piece := range WordTones(text, format, sy.WordMs, sy.GapMs) {
			if !yield(piece, nil) {
				return
			}
			if pause > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(pause):
				}
			}
		}
	}
}
