package deepgram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/coder/websocket"
)

const testKey = "test-key"

// recording is what a fake Deepgram was sent.
type recording struct {
	mu         sync.Mutex
	queries    []url.Values
	audioBytes int
	messages   []map[string]string
}

func (r *recording) controls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var kinds []string
	for _, m := range r.messages {
		kinds = append(kinds, m["type"])
	}
	return kinds
}

// fakeDeepgram serves handle behind Deepgram's key check; it returns the ws:// base URL.
func fakeDeepgram(t *testing.T, handle func(context.Context, *websocket.Conn, *recording)) (string, *recording) {
	rec := &recording{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token "+testKey {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
		rec.mu.Lock()
		rec.queries = append(rec.queries, r.URL.Query())
		rec.mu.Unlock()
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		handle(r.Context(), conn, rec)
	}))
	t.Cleanup(server.Close)
	return "ws" + strings.TrimPrefix(server.URL, "http"), rec
}

func send(ctx context.Context, conn *websocket.Conn, v any) {
	raw, _ := json.Marshal(v)
	_ = conn.Write(ctx, websocket.MessageText, raw)
}

func resultsMessage(text string, final bool, start, duration float64) map[string]any {
	return map[string]any{
		"type": "Results", "is_final": final, "start": start, "duration": duration,
		"channel": map[string]any{"alternatives": []any{map[string]any{"transcript": text, "confidence": 0.93}}},
	}
}

// listen hears "hello" after 400 ms of audio, then "hello world" once finalized.
func listen(ctx context.Context, conn *websocket.Conn, rec *recording) {
	received, spoke := 0, false
	for {
		kind, raw, err := conn.Read(ctx)
		if err != nil {
			return
		}
		if kind == websocket.MessageBinary {
			received += len(raw)
			rec.mu.Lock()
			rec.audioBytes += len(raw)
			rec.mu.Unlock()
			if !spoke && received >= 24000*2*4/10 {
				spoke = true
				send(ctx, conn, map[string]any{"type": "SpeechStarted", "timestamp": 0.1})
				send(ctx, conn, resultsMessage("hello", false, 0.1, 0.3))
			}
			continue
		}
		var m map[string]string
		_ = json.Unmarshal(raw, &m)
		rec.mu.Lock()
		rec.messages = append(rec.messages, m)
		rec.mu.Unlock()
		switch m["type"] {
		case "Finalize":
			if spoke {
				send(ctx, conn, resultsMessage("hello world", true, 0.1, 0.8))
				send(ctx, conn, map[string]any{"type": "UtteranceEnd", "last_word_end": 0.9})
			}
		case "CloseStream":
			send(ctx, conn, map[string]any{"type": "Metadata", "request_id": "r1"})
			conn.Close(websocket.StatusNormalClosure, "")
			return
		}
	}
}

// speak answers Flush with 20 ms of audio per character, in odd-sized pieces.
func speak(ctx context.Context, conn *websocket.Conn, rec *recording) {
	send(ctx, conn, map[string]string{"type": "Metadata", "request_id": "r1"})
	text := ""
	for {
		_, raw, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var m map[string]string
		_ = json.Unmarshal(raw, &m)
		rec.mu.Lock()
		rec.messages = append(rec.messages, m)
		rec.mu.Unlock()
		switch m["type"] {
		case "Speak":
			text += m["text"]
		case "Flush":
			audio := []byte(strings.Repeat("\x03\x04", len(text)*24000/50))
			for start := 0; start < len(audio); start += 4001 {
				_ = conn.Write(ctx, websocket.MessageBinary, audio[start:min(start+4001, len(audio))])
			}
			send(ctx, conn, map[string]any{"type": "Flushed", "sequence_id": 0})
		case "Close":
			return
		}
	}
}
