package audiostreamin

import (
	"context"
	"errors"
	"iter"
)

// Frame is what a provider stream yields: one of the *Frame types below.
type Frame interface{ frame() }

type SpeechStartedFrame struct{ AtMs int }

type PartialFrame struct {
	Text           string
	StartMs, EndMs int
}

type FinalFrame struct {
	Text           string
	StartMs, EndMs int
	Confidence     *float64
}

type UtteranceEndFrame struct{}

func (SpeechStartedFrame) frame() {}
func (PartialFrame) frame()       {}
func (FinalFrame) frame()         {}
func (UtteranceEndFrame) frame()  {}

// ErrProviderUnavailable is wrapped by a provider that refuses, e.g. without a key.
var ErrProviderUnavailable = errors.New("the transcription provider is unavailable")

// TranscriberStream is one provider stream: audio in, frames out.
type TranscriberStream interface {
	Send(ctx context.Context, audio []byte) error
	// Keepalive is called while no audio arrives.
	Keepalive(ctx context.Context) error
	// Finish flushes; Frames ends after the last results.
	Finish(ctx context.Context) error
	// Close abandons pending results.
	Close() error
	Frames(ctx context.Context) iter.Seq2[Frame, error]
}
