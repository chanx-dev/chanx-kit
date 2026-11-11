package openai

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/coder/websocket"
	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/chanxtest"

	"go.chanx.dev/kit/kits/audiostreamin"
)

const testKey = "test-key"

type recording struct {
	mu         sync.Mutex
	sessions   []map[string]any
	audioBytes int
	commits    int
}

// fakeRealtime hears "hello" after 400 ms of audio, then " world" and the transcript on commit.
func fakeRealtime(t *testing.T) (string, *recording) {
	rec := &recording{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testKey {
			http.Error(w, "invalid api key", http.StatusUnauthorized)
			return
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx := r.Context()
		send := func(v any) {
			raw, _ := json.Marshal(v)
			_ = conn.Write(ctx, websocket.MessageText, raw)
		}
		received, spoke, item := 0, false, 0
		for {
			_, raw, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var e struct {
				Type    string         `json:"type"`
				Session map[string]any `json:"session"`
				Audio   string         `json:"audio"`
			}
			_ = json.Unmarshal(raw, &e)
			id := fmt.Sprintf("i%d", item)
			rec.mu.Lock()
			switch e.Type {
			case "session.update":
				rec.sessions = append(rec.sessions, e.Session)
			case "input_audio_buffer.append":
				audio, _ := base64.StdEncoding.DecodeString(e.Audio)
				received += len(audio)
				rec.audioBytes += len(audio)
				if !spoke && received >= 24000*2*4/10 {
					spoke = true
					send(map[string]any{"type": "conversation.item.input_audio_transcription.delta", "item_id": id, "delta": "hello"})
				}
			case "input_audio_buffer.commit":
				rec.commits++
				if !spoke {
					send(map[string]any{"type": "error", "error": map[string]any{"code": "input_audio_buffer_commit_empty", "message": "buffer too small"}})
					break
				}
				send(map[string]any{"type": "conversation.item.input_audio_transcription.delta", "item_id": id, "delta": " world"})
				send(map[string]any{"type": "conversation.item.input_audio_transcription.completed", "item_id": id, "transcript": "hello world"})
				item++
				received, spoke = 0, false
			}
			rec.mu.Unlock()
		}
	}))
	t.Cleanup(server.Close)
	return "ws" + strings.TrimPrefix(server.URL, "http"), rec
}

type sttConsumer struct {
	topic audiostreamin.TranscriberTopic
}

func (c sttConsumer) Topics() []chanx.TopicHandler { return []chanx.TopicHandler{c.topic} }

var at = chanxtest.Topic("transcribe:s")

func utterance(c *chanxtest.Client, chunks int, language *string) {
	c.Send(audiostreamin.AudioStartMessage{Payload: audiostreamin.AudioStart{
		Encoding: audiostreamin.PCM16, SampleRate: 24000, Channels: 1, Language: language,
	}}, at)
	for i := range chunks {
		c.Send(audiostreamin.AudioChunkMessage{Payload: audiostreamin.AudioChunk{Index: i, Data: make([]byte, 4800)}}, at)
	}
	c.Send(audiostreamin.AudioEndMessage{}, at)
}

func join(t *testing.T, tr Transcriber) *chanxtest.Client {
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws", sttConsumer{tr.Topic()})
	c := chanxtest.Connect(t, engine, "/ws")
	c.Subscribe("transcribe:s")
	chanxtest.ReceiveUntilMessage[audiostreamin.AudioConfigMessage](c)
	return c
}

func TestTheSessionIsConfiguredForTranscription(t *testing.T) {
	address, rec := fakeRealtime(t)
	c := join(t, Transcriber{APIKey: testKey, URL: address})
	language := "en"
	utterance(c, 5, &language)
	var partials []string
	for _, f := range c.ReceiveUntil("transcript_final") {
		if f.Action() == "transcript_partial" {
			partials = append(partials, f.Payload().(map[string]any)["text"].(string))
		}
	}
	if !reflect.DeepEqual(partials, []string{"hello", "hello world"}) {
		t.Fatalf("partials %v", partials)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	input := rec.sessions[0]["audio"].(map[string]any)["input"].(map[string]any)
	want := map[string]any{"model": "gpt-live-transcribe", "language": "en"}
	if rec.sessions[0]["type"] != "transcription" || !reflect.DeepEqual(input["transcription"], want) ||
		input["turn_detection"] != nil || rec.commits != 1 {
		t.Fatalf("session %v, %d commits", rec.sessions[0], rec.commits)
	}
}

func TestEndingWithoutSpeechIsNotAnError(t *testing.T) {
	address, _ := fakeRealtime(t)
	c := join(t, Transcriber{APIKey: testKey, URL: address})
	utterance(c, 1, nil)
	utterance(c, 5, nil)
	frames := c.ReceiveUntil("transcriber_error", "transcript_final")
	if last := frames[len(frames)-1]; last.Action() != "transcript_final" {
		t.Fatalf("got %s", last)
	}
}
