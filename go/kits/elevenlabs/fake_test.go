package elevenlabs

import (
	"context"
	"encoding/base64"
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

// recording is what a fake ElevenLabs was sent.
type recording struct {
	mu         sync.Mutex
	paths      []string
	queries    []url.Values
	audioBytes int
	commits    int
	messages   []map[string]any
}

func (r *recording) lock(f func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f()
}

// fakeElevenLabs serves handle behind the xi-api-key check; it returns the ws:// base URL.
func fakeElevenLabs(t *testing.T, handle func(context.Context, *websocket.Conn, *recording)) (string, *recording) {
	rec := &recording{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("xi-api-key") != testKey {
			http.Error(w, "invalid api key", http.StatusUnauthorized)
			return
		}
		rec.lock(func() {
			rec.paths = append(rec.paths, r.URL.Path)
			rec.queries = append(rec.queries, r.URL.Query())
		})
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

// scribe hears "hello" after 400 ms of audio, and commits "hello world".
func scribe(ctx context.Context, conn *websocket.Conn, rec *recording) {
	send(ctx, conn, map[string]string{"message_type": "session_started", "session_id": "s"})
	received, spoke := 0, false
	for {
		_, raw, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var m struct {
			Audio  string `json:"audio_base_64"`
			Commit bool   `json:"commit"`
		}
		_ = json.Unmarshal(raw, &m)
		audio, _ := base64.StdEncoding.DecodeString(m.Audio)
		received += len(audio)
		rec.lock(func() { rec.audioBytes += len(audio) })
		if !spoke && received >= 24000*2*4/10 {
			spoke = true
			send(ctx, conn, map[string]string{"message_type": "partial_transcript", "text": "hello"})
		}
		if m.Commit {
			rec.lock(func() { rec.commits++ })
			if spoke {
				send(ctx, conn, map[string]string{"message_type": "committed_transcript", "text": "hello world"})
			}
		}
	}
}

// streamInput answers the end of input with 20 ms of audio per character, in odd-sized pieces.
func streamInput(ctx context.Context, conn *websocket.Conn, rec *recording) {
	spoken := ""
	for {
		_, raw, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		rec.lock(func() { rec.messages = append(rec.messages, m) })
		text, _ := m["text"].(string)
		if text != "" {
			spoken += text
			continue
		}
		audio := []byte(strings.Repeat("\x01\x02", len(strings.TrimSpace(spoken))*24000/50))
		for start := 0; start < len(audio); start += 3001 {
			piece := base64.StdEncoding.EncodeToString(audio[start:min(start+3001, len(audio))])
			send(ctx, conn, map[string]any{"audio": piece, "isFinal": nil})
		}
		send(ctx, conn, map[string]any{"audio": nil, "isFinal": true})
		return
	}
}
