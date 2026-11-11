package audiostreamout

import (
	"github.com/google/jsonschema-go/jsonschema"
	chanx "go.chanx.dev/v2"
)

// SpeechFormat is raw PCM16 little-endian audio, interleaved when there are several channels.
type SpeechFormat struct {
	Encoding   string `json:"encoding,omitempty"`
	SampleRate int    `json:"sample_rate,omitempty"`
	Channels   int    `json:"channels,omitempty"`
}

// DefaultSpeechFormat is 24 kHz mono PCM16.
var DefaultSpeechFormat = SpeechFormat{Encoding: "pcm16", SampleRate: 24000, Channels: 1}

// pcm16Only makes a schema's encoding the "pcm16" literal, as the contract has it.
func pcm16Only(s *jsonschema.Schema) {
	s.Properties["encoding"] = &jsonschema.Schema{Type: "string", Const: jsonschema.Ptr[any]("pcm16"), Default: []byte(`"pcm16"`)}
}

type Speak struct {
	Text        string  `json:"text"`
	Voice       *string `json:"voice,omitempty"`
	UtteranceID *string `json:"utterance_id,omitempty"`
}

type SpeakMessage struct {
	chanx.BaseMessage `action:"speak" description:"Client asks for text to be spoken. Queued behind what is already speaking."`
	Payload           Speak `json:"payload"`
}

type SpeakClearMessage struct {
	chanx.BaseMessage `action:"speak_clear" description:"Client asks to stop: the utterance playing and everything queued."`
}

// PlaybackMark is how much of an utterance was actually heard, in milliseconds.
type PlaybackMark struct {
	UtteranceID string `json:"utterance_id"`
	PlayedMs    int    `json:"played_ms"`
}

type PlaybackMarkMessage struct {
	chanx.BaseMessage `action:"playback_mark" description:"Client reports playback progress, so the server knows what was heard."`
	Payload           PlaybackMark `json:"payload"`
}

type SpeechAudioStart struct {
	Encoding    string `json:"encoding,omitempty"`
	SampleRate  int    `json:"sample_rate,omitempty"`
	Channels    int    `json:"channels,omitempty"`
	UtteranceID string `json:"utterance_id"`
	Text        string `json:"text"`
}

func (SpeechAudioStart) JSONSchema(s *jsonschema.Schema) { pcm16Only(s) }

type SpeechAudioStartMessage struct {
	chanx.BaseMessage `action:"audio_start" description:"An utterance begins, in this format."`
	Payload           SpeechAudioStart `json:"payload"`
}

// SpeechAudioChunk counts Index from 0 per utterance, so a replayed chunk can be recognised.
type SpeechAudioChunk struct {
	UtteranceID string `json:"utterance_id"`
	Index       int    `json:"index"`
	Data        []byte `json:"data"`
}

type SpeechAudioChunkMessage struct {
	chanx.BaseMessage `action:"audio_chunk" description:"A piece of the utterance's audio."`
	Payload           SpeechAudioChunk `json:"payload"`
}

type SpeechAudioEnd struct {
	UtteranceID string `json:"utterance_id"`
	DurationMs  int    `json:"duration_ms"`
}

type SpeechAudioEndMessage struct {
	chanx.BaseMessage `action:"audio_end" description:"The utterance's audio is complete."`
	Payload           SpeechAudioEnd `json:"payload"`
}

// SpeechClear names the utterance that was playing, if any.
type SpeechClear struct {
	UtteranceID *string `json:"utterance_id,omitempty"`
}

type SpeechClearMessage struct {
	chanx.BaseMessage `action:"clear" description:"Stop playing now and drop anything buffered: an interruption."`
	Payload           SpeechClear `json:"payload"`
}

type ErrorCode string

const (
	ProviderUnavailable ErrorCode = "provider_unavailable"
	BadRequest          ErrorCode = "bad_request"
	ProviderFailed      ErrorCode = "provider_failed"
)

func (ErrorCode) JSONSchema(s *jsonschema.Schema) {
	*s = jsonschema.Schema{Type: "string", Enum: []any{"provider_unavailable", "bad_request", "provider_failed"}}
}

type SynthesizerError struct {
	Code        ErrorCode `json:"code"`
	Message     string    `json:"message"`
	UtteranceID *string   `json:"utterance_id,omitempty"`
}

type SynthesizerErrorMessage struct {
	chanx.BaseMessage `action:"synthesizer_error" description:"An utterance could not be spoken."`
	Payload           SynthesizerError `json:"payload"`
}
