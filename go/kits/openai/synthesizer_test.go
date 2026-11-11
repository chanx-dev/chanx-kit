package openai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"go.chanx.dev/kit/kits/audiostreamout"
)

// fakeSpeech is OpenAI's speech endpoint: 20 ms of PCM per input character.
func fakeSpeech(t *testing.T) (string, *[]map[string]any) {
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testKey {
			http.Error(w, `{"error":{"message":"bad key"}}`, http.StatusUnauthorized)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		requests = append(requests, body)
		w.Header().Set("Content-Type", "audio/pcm")
		_, _ = w.Write([]byte(strings.Repeat("\x05\x06", len(body["input"].(string))*24000/50)))
	}))
	t.Cleanup(server.Close)
	return server.URL, &requests
}

func collect(s Synthesizer, format audiostreamout.SpeechFormat) ([]byte, error) {
	var audio []byte
	for piece, err := range s.Synthesize(context.Background(), "Hello", "nova", format) {
		if err != nil {
			return audio, err
		}
		audio = append(audio, piece...)
	}
	return audio, nil
}

func TestTheRequestAsksForRawPCM(t *testing.T) {
	address, requests := fakeSpeech(t)
	audio, err := collect(Synthesizer{APIKey: testKey, URL: address, Instructions: "Speak warmly."}, audiostreamout.DefaultSpeechFormat)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"model": "gpt-4o-mini-tts", "voice": "nova", "input": "Hello", "response_format": "pcm", "instructions": "Speak warmly."}
	if !reflect.DeepEqual((*requests)[0], want) || len(audio) != 2*(5*24000/50) {
		t.Fatalf("request %v, %d bytes", (*requests)[0], len(audio))
	}
}

func TestAWrongKeyIsUnavailableNotACrash(t *testing.T) {
	address, _ := fakeSpeech(t)
	_, err := collect(Synthesizer{APIKey: "nope", URL: address}, audiostreamout.DefaultSpeechFormat)
	if !errors.Is(err, audiostreamout.ErrProviderUnavailable) || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err %v", err)
	}
}

func TestOnly24kHzMonoIsOffered(t *testing.T) {
	_, err := collect(Synthesizer{APIKey: testKey}, audiostreamout.SpeechFormat{SampleRate: 16000, Channels: 1})
	if !errors.Is(err, audiostreamout.ErrProviderUnavailable) {
		t.Fatalf("err %v", err)
	}
}
