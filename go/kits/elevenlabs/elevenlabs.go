// Package elevenlabs puts ElevenLabs behind the voice contracts: Scribe speech-to-text, streaming text-to-speech.
package elevenlabs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"time"

	"github.com/coder/websocket"
)

// PCMRates are the mono PCM sample rates ElevenLabs takes and produces.
var PCMRates = []int{8000, 16000, 22050, 24000, 44100, 48000}

// key is apiKey, or ELEVENLABS_API_KEY when used, so a .env loaded after start works.
func key(apiKey string) string {
	if apiKey != "" {
		return apiKey
	}
	return os.Getenv("ELEVENLABS_API_KEY")
}

// ConnectTimeout bounds opening a connection, so a stalled provider is reported.
var ConnectTimeout = 10 * time.Second

// dial connects with the key, turning a refusal into unavailable.
func dial(ctx context.Context, address, apiKey string, unavailable error) (*websocket.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, ConnectTimeout)
	defer cancel()
	conn, resp, err := websocket.Dial(ctx, address, &websocket.DialOptions{
		HTTPHeader: http.Header{"xi-api-key": {apiKey}},
	})
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("%w: ElevenLabs refused the connection (%d)", unavailable, resp.StatusCode)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: ElevenLabs did not answer within %s", unavailable, ConnectTimeout)
		}
		return nil, fmt.Errorf("%w: ElevenLabs is unreachable: %v", unavailable, err)
	}
	conn.SetReadLimit(-1)
	return conn, nil
}

func supported(rate, channels int) bool { return slices.Contains(PCMRates, rate) && channels == 1 }

func or[T comparable](value, fallback T) T {
	var zero T
	if value == zero {
		return fallback
	}
	return value
}
