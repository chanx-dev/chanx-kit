package deepgram

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/url"
	"slices"
	"strconv"

	"github.com/coder/websocket"

	"go.chanx.dev/kit/kits/audiostreamout"
)

// PCMRates are the sample rates Aura produces as PCM.
var PCMRates = []int{8000, 16000, 24000, 32000, 48000}

// Synthesizer is Deepgram Aura text-to-speech.
type Synthesizer struct {
	// APIKey defaults to DEEPGRAM_API_KEY.
	APIKey string
	URL    string
	// Voice is the default Aura model.
	Voice string
}

// Topic is a synthesizer topic on Deepgram.
func (s Synthesizer) Topic() audiostreamout.SynthesizerTopic {
	return audiostreamout.SynthesizerTopic{Synthesize: s.Synthesize, ProviderAvailable: s.Available}
}

func (s Synthesizer) Available() bool { return key(s.APIKey) != "" }

func (s Synthesizer) SpeakURL(voice string, format audiostreamout.SpeechFormat) (string, error) {
	if !slices.Contains(PCMRates, format.SampleRate) || format.Channels != 1 {
		return "", fmt.Errorf("%w: Deepgram produces mono PCM at %v Hz", audiostreamout.ErrProviderUnavailable, PCMRates)
	}
	query := url.Values{
		"model":       {or(voice, or(s.Voice, "aura-2-thalia-en"))},
		"encoding":    {"linear16"},
		"sample_rate": {strconv.Itoa(format.SampleRate)},
	}
	return or(s.URL, "wss://api.deepgram.com/v1/speak") + "?" + query.Encode(), nil
}

func (s Synthesizer) Synthesize(ctx context.Context, text, voice string, format audiostreamout.SpeechFormat) iter.Seq2[[]byte, error] {
	return func(yield func([]byte, error) bool) {
		apiKey := key(s.APIKey)
		if apiKey == "" {
			yield(nil, fmt.Errorf("%w: DEEPGRAM_API_KEY is not set", audiostreamout.ErrProviderUnavailable))
			return
		}
		address, err := s.SpeakURL(voice, format)
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

		speak, _ := json.Marshal(map[string]string{"type": "Speak", "text": text})
		for _, m := range [][]byte{speak, []byte(`{"type":"Flush"}`)} {
			if err := conn.Write(ctx, websocket.MessageText, m); err != nil {
				yield(nil, err)
				return
			}
		}
		for {
			kind, raw, err := conn.Read(ctx)
			if err != nil {
				if ctx.Err() == nil {
					yield(nil, err)
				}
				return
			}
			if kind == websocket.MessageBinary {
				if !yield(raw, nil) {
					return
				}
				continue
			}
			var event struct {
				Type        string `json:"type"`
				Description string `json:"description"`
			}
			_ = json.Unmarshal(raw, &event)
			switch event.Type {
			case "Flushed":
				_ = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"Close"}`))
				return
			case "Warning":
				yield(nil, fmt.Errorf("deepgram: %s", event.Description))
				return
			}
		}
	}
}
