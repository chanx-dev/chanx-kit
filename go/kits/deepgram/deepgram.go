// Package deepgram puts Deepgram behind the voice contracts: Nova speech-to-text, Aura text-to-speech.
package deepgram

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/coder/websocket"
)

// key is apiKey, or DEEPGRAM_API_KEY when used, so a .env loaded after start works.
func key(apiKey string) string {
	if apiKey != "" {
		return apiKey
	}
	return os.Getenv("DEEPGRAM_API_KEY")
}

// ConnectTimeout bounds opening a connection, so a stalled provider is reported.
var ConnectTimeout = 10 * time.Second

// dial connects with the key, turning a refusal into unavailable.
func dial(ctx context.Context, address, apiKey string, unavailable error) (*websocket.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, ConnectTimeout)
	defer cancel()
	conn, resp, err := websocket.Dial(ctx, address, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": {"Token " + apiKey}},
	})
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("%w: Deepgram refused the connection (%d)", unavailable, resp.StatusCode)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: Deepgram did not answer within %s", unavailable, ConnectTimeout)
		}
		return nil, fmt.Errorf("%w: Deepgram is unreachable: %v", unavailable, err)
	}
	conn.SetReadLimit(-1)
	return conn, nil
}

func closedNormally(err error) bool {
	return websocket.CloseStatus(err) == websocket.StatusNormalClosure
}

func or[T comparable](value, fallback T) T {
	var zero T
	if value == zero {
		return fallback
	}
	return value
}
