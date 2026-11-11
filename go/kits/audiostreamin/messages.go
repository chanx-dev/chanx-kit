package audiostreamin

import (
	"github.com/google/jsonschema-go/jsonschema"
	chanx "go.chanx.dev/v2"
)

// Encoding is raw PCM16 little-endian, interleaved when there are several channels.
type Encoding string

const PCM16 Encoding = "pcm16"

// pcm16Only makes a schema's encoding the "pcm16" literal, as the contract has it.
func pcm16Only(s *jsonschema.Schema) {
	s.Properties["encoding"] = &jsonschema.Schema{Type: "string", Const: jsonschema.Ptr[any]("pcm16"), Default: []byte(`"pcm16"`)}
}

// AudioConfig is the format the server wants, and how much audio to put in each chunk.
type AudioConfig struct {
	Encoding   Encoding `json:"encoding,omitempty"`
	SampleRate int      `json:"sample_rate,omitempty"`
	Channels   int      `json:"channels,omitempty"`
	ChunkMs    int      `json:"chunk_ms,omitempty"`
}

func (AudioConfig) JSONSchema(s *jsonschema.Schema) { pcm16Only(s) }

// DefaultAudioConfig is 24 kHz mono PCM16 in 100 ms chunks.
var DefaultAudioConfig = AudioConfig{Encoding: PCM16, SampleRate: 24000, Channels: 1, ChunkMs: 100}

type AudioConfigMessage struct {
	chanx.BaseMessage `action:"audio_config" description:"Sent on subscribe: capture to this format."`
	Payload           AudioConfig `json:"payload"`
}

// AudioConfigRequestMessage asks for the capture format again, without resubscribing.
type AudioConfigRequestMessage struct {
	chanx.BaseMessage `action:"audio_config_request" description:"Client asks for the capture format again, without resubscribing."`
}

// AudioStart is the format actually captured, which must match audio_config.
type AudioStart struct {
	Encoding   Encoding `json:"encoding,omitempty"`
	SampleRate int      `json:"sample_rate,omitempty"`
	Channels   int      `json:"channels,omitempty"`
	Language   *string  `json:"language,omitempty"`
}

func (AudioStart) JSONSchema(s *jsonschema.Schema) { pcm16Only(s) }

type AudioStartMessage struct {
	chanx.BaseMessage `action:"audio_start" description:"Client begins a stream of audio."`
	Payload           AudioStart `json:"payload"`
}

// AudioChunk counts Index from 0 for each audio_start, so a gap can be detected.
type AudioChunk struct {
	Index int    `json:"index"`
	Data  []byte `json:"data"`
}

type AudioChunkMessage struct {
	chanx.BaseMessage `action:"audio_chunk" description:"A piece of the stream."`
	Payload           AudioChunk `json:"payload"`
}

type AudioEndMessage struct {
	chanx.BaseMessage `action:"audio_end" description:"The stream is over: flush what the provider holds, then close it."`
}

type SpeechStarted struct {
	AtMs int `json:"at_ms"`
}

type SpeechStartedMessage struct {
	chanx.BaseMessage `action:"speech_started" description:"The provider heard speech begin, for barge-in and listening indicators."`
	Payload           SpeechStarted `json:"payload"`
}

// Transcript times are milliseconds from the audio_start of this stream.
type Transcript struct {
	UtteranceID string `json:"utterance_id"`
	Text        string `json:"text"`
	StartMs     int    `json:"start_ms"`
	EndMs       int    `json:"end_ms"`
}

type TranscriptPartialMessage struct {
	chanx.BaseMessage `action:"transcript_partial" description:"The segment being spoken, so far. Replaces the previous partial of the utterance."`
	Payload           Transcript `json:"payload"`
}

type TranscriptFinal struct {
	UtteranceID string   `json:"utterance_id"`
	Text        string   `json:"text"`
	StartMs     int      `json:"start_ms"`
	EndMs       int      `json:"end_ms"`
	Confidence  *float64 `json:"confidence,omitempty"`
}

type TranscriptFinalMessage struct {
	chanx.BaseMessage `action:"transcript_final" description:"A segment that will not change. An utterance is its finals in order."`
	Payload           TranscriptFinal `json:"payload"`
}

type UtteranceEnd struct {
	UtteranceID string `json:"utterance_id"`
}

type UtteranceEndMessage struct {
	chanx.BaseMessage `action:"utterance_end" description:"The speaker paused: the utterance is complete."`
	Payload           UtteranceEnd `json:"payload"`
}

// ErrorCode says what went wrong; every code but AudioGap ends the stream.
type ErrorCode string

const (
	ProviderUnavailable ErrorCode = "provider_unavailable"
	LimitReached        ErrorCode = "limit_reached"
	BadFormat           ErrorCode = "bad_format"
	AudioGap            ErrorCode = "audio_gap"
	NotStarted          ErrorCode = "not_started"
)

func (ErrorCode) JSONSchema(s *jsonschema.Schema) {
	*s = jsonschema.Schema{Type: "string", Enum: []any{"provider_unavailable", "limit_reached", "bad_format", "audio_gap", "not_started"}}
}

type TranscriberError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

type TranscriberErrorMessage struct {
	chanx.BaseMessage `action:"transcriber_error" description:"Something went wrong. Every code except audio_gap ends the stream."`
	Payload           TranscriberError `json:"payload"`
}
