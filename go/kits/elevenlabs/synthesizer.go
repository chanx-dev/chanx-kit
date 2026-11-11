package elevenlabs

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"iter"
	"net/url"
	"strconv"
	"strings"

	"github.com/coder/websocket"

	"go.chanx.dev/kit/kits/audiostreamout"
)

// Refusal says what to change when ElevenLabs refuses in-band.
func Refusal(code, voice string) string {
	hints := map[string]string{
		"payment_required": "voice " + voice + " is not available on this plan; use a premade voice",
		"quota_exceeded":   "the account's character quota is used up",
	}
	if hint, ok := hints[code]; ok {
		return fmt.Sprintf("ElevenLabs: %s (%s)", code, hint)
	}
	return "ElevenLabs: " + code
}

// Synthesizer is ElevenLabs streaming text-to-speech.
type Synthesizer struct {
	// APIKey defaults to ELEVENLABS_API_KEY.
	APIKey string
	// URL has a {voice_id} placeholder.
	URL string
	// Voice defaults to Sarah, premade on every plan; retired voices such as Rachel cost money.
	Voice         string
	ModelID       string
	VoiceSettings map[string]any
	Options       map[string]string
}

// Topic is a synthesizer topic on ElevenLabs.
func (s Synthesizer) Topic() audiostreamout.SynthesizerTopic {
	return audiostreamout.SynthesizerTopic{Synthesize: s.Synthesize, ProviderAvailable: s.Available}
}

func (s Synthesizer) Available() bool { return key(s.APIKey) != "" }

func (s Synthesizer) StreamURL(voice string, format audiostreamout.SpeechFormat) (string, error) {
	if !supported(format.SampleRate, format.Channels) {
		return "", fmt.Errorf("%w: ElevenLabs produces mono PCM at %v Hz", audiostreamout.ErrProviderUnavailable, PCMRates)
	}
	query := url.Values{
		"model_id":      {or(s.ModelID, "eleven_flash_v2_5")},
		"output_format": {"pcm_" + strconv.Itoa(format.SampleRate)},
	}
	for k, v := range s.Options {
		query.Set(k, v)
	}
	base := or(s.URL, "wss://api.elevenlabs.io/v1/text-to-speech/{voice_id}/stream-input")
	return strings.Replace(base, "{voice_id}", url.PathEscape(voice), 1) + "?" + query.Encode(), nil
}

func (s Synthesizer) settings() map[string]any {
	if s.VoiceSettings != nil {
		return s.VoiceSettings
	}
	return map[string]any{"stability": 0.5, "similarity_boost": 0.75}
}

func (s Synthesizer) Synthesize(ctx context.Context, text, voice string, format audiostreamout.SpeechFormat) iter.Seq2[[]byte, error] {
	return func(yield func([]byte, error) bool) {
		apiKey := key(s.APIKey)
		if apiKey == "" {
			yield(nil, fmt.Errorf("%w: ELEVENLABS_API_KEY is not set", audiostreamout.ErrProviderUnavailable))
			return
		}
		voice = or(voice, or(s.Voice, "EXAVITQu4vr4xnSDxMaL"))
		address, err := s.StreamURL(voice, format)
		if err != nil {
			yield(nil, err)
			return
		}
		conn, err := dial(ctx, address, apiKey, audiostreamout.ErrProviderUnavailable)
		if err != nil {
			yield(nil, err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")

		// The opening text is a single space; empty text ends the input.
		for _, m := range []map[string]any{
			{"text": " ", "voice_settings": s.settings()},
			{"text": text + " ", "flush": true},
			{"text": ""},
		} {
			raw, _ := json.Marshal(m)
			if err := conn.Write(ctx, websocket.MessageText, raw); err != nil {
				yield(nil, err)
				return
			}
		}
		for {
			_, raw, err := conn.Read(ctx)
			if err != nil {
				if ctx.Err() == nil {
					yield(nil, err)
				}
				return
			}
			var m struct {
				Audio   string `json:"audio"`
				IsFinal bool   `json:"isFinal"`
				Error   string `json:"error"`
			}
			_ = json.Unmarshal(raw, &m)
			if m.Error != "" {
				yield(nil, fmt.Errorf("%w: %s", audiostreamout.ErrProviderUnavailable, Refusal(m.Error, voice)))
				return
			}
			if m.Audio != "" {
				audio, err := base64.StdEncoding.DecodeString(m.Audio)
				if err != nil || !yield(audio, nil) {
					return
				}
			}
			if m.IsFinal {
				return
			}
		}
	}
}
