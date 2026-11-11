package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"

	"go.chanx.dev/kit/kits/audiostreamout"
)

// Synthesizer is OpenAI text-to-speech as streamed PCM.
type Synthesizer struct {
	// APIKey defaults to OPENAI_API_KEY.
	APIKey string
	URL    string
	Model  string
	Voice  string
	// Instructions set tone, pace and accent.
	Instructions string
	// Client defaults to http.DefaultClient.
	Client *http.Client
}

// Topic is a synthesizer topic on OpenAI.
func (s Synthesizer) Topic() audiostreamout.SynthesizerTopic {
	return audiostreamout.SynthesizerTopic{Synthesize: s.Synthesize, ProviderAvailable: s.Available}
}

func (s Synthesizer) Available() bool { return key(s.APIKey) != "" }

func (s Synthesizer) Synthesize(ctx context.Context, text, voice string, format audiostreamout.SpeechFormat) iter.Seq2[[]byte, error] {
	return func(yield func([]byte, error) bool) {
		apiKey := key(s.APIKey)
		if apiKey == "" {
			yield(nil, fmt.Errorf("%w: OPENAI_API_KEY is not set", audiostreamout.ErrProviderUnavailable))
			return
		}
		// The pcm response format is fixed: 24 kHz, 16-bit, mono.
		if format.SampleRate != PCMRate || format.Channels != 1 {
			yield(nil, fmt.Errorf("%w: OpenAI speech is PCM16 mono at 24 kHz", audiostreamout.ErrProviderUnavailable))
			return
		}
		body := map[string]any{
			"model":           or(s.Model, "gpt-4o-mini-tts"),
			"voice":           or(voice, or(s.Voice, "alloy")),
			"input":           text,
			"response_format": "pcm",
		}
		if s.Instructions != "" {
			body["instructions"] = s.Instructions
		}
		raw, _ := json.Marshal(body)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, or(s.URL, "https://api.openai.com/v1/audio/speech"), bytes.NewReader(raw))
		if err != nil {
			yield(nil, err)
			return
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err := or(s.Client, http.DefaultClient).Do(req)
		if err != nil {
			yield(nil, fmt.Errorf("%w: OpenAI is unreachable: %v", audiostreamout.ErrProviderUnavailable, err))
			return
		}
		defer resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			yield(nil, fmt.Errorf("%w: OpenAI refused the request (%d)", audiostreamout.ErrProviderUnavailable, resp.StatusCode))
			return
		case resp.StatusCode >= 400:
			detail, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
			yield(nil, fmt.Errorf("OpenAI speech failed (%d): %s", resp.StatusCode, detail))
			return
		}
		buf := make([]byte, 32*1024)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 && !yield(bytes.Clone(buf[:n]), nil) {
				return
			}
			if err == io.EOF {
				return
			}
			if err != nil {
				if ctx.Err() == nil {
					yield(nil, err)
				}
				return
			}
		}
	}
}
