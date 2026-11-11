package elevenlabs

import (
	"context"
	"errors"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/chanxtest"

	"go.chanx.dev/kit/kits/audiostreamout"
)

type ttsConsumer struct {
	topic audiostreamout.SynthesizerTopic
}

func (c ttsConsumer) Topics() []chanx.TopicHandler { return []chanx.TopicHandler{c.topic} }

func say(t *testing.T, s Synthesizer, speak audiostreamout.Speak) *chanxtest.Client {
	t.Helper()
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws", ttsConsumer{s.Topic()})
	c := chanxtest.Connect(t, engine, "/ws")
	c.Subscribe("speak:s")
	c.Send(audiostreamout.SpeakMessage{Payload: speak}, chanxtest.Topic("speak:s"))
	return c
}

func TestTheVoiceFormatAndTextReachElevenLabs(t *testing.T) {
	address, rec := fakeElevenLabs(t, streamInput)
	voice := "voice-9"
	c := say(t, Synthesizer{APIKey: testKey, URL: address + "/v1/text-to-speech/{voice_id}/stream-input"},
		audiostreamout.Speak{Text: "Xin chao", Voice: &voice})
	end, _ := chanxtest.ReceiveUntilMessage[audiostreamout.SpeechAudioEndMessage](c)

	if end.Payload.DurationMs != 160 {
		t.Fatalf("duration %d ms", end.Payload.DurationMs)
	}
	rec.lock(func() {
		if !reflect.DeepEqual(rec.paths, []string{"/v1/text-to-speech/voice-9/stream-input"}) {
			t.Fatalf("paths %v", rec.paths)
		}
		if q := rec.queries[0]; q.Get("output_format") != "pcm_24000" || q.Get("model_id") != "eleven_flash_v2_5" {
			t.Fatalf("query %v", q)
		}
		var texts []any
		for _, m := range rec.messages {
			texts = append(texts, m["text"])
		}
		if !reflect.DeepEqual(texts, []any{" ", "Xin chao ", ""}) || rec.messages[0]["voice_settings"] == nil {
			t.Fatalf("messages %v", rec.messages)
		}
	})
}

func TestAWrongKeyIsReported(t *testing.T) {
	address, _ := fakeElevenLabs(t, streamInput)
	c := say(t, Synthesizer{APIKey: "nope", URL: address + "/{voice_id}"}, audiostreamout.Speak{Text: "hi"})
	m, _ := chanxtest.ReceiveUntilMessage[audiostreamout.SynthesizerErrorMessage](c)
	if m.Payload.Code != audiostreamout.ProviderUnavailable || !strings.Contains(m.Payload.Message, "401") {
		t.Fatalf("error %+v", m.Payload)
	}
}

func TestAnUnsupportedFormatIsRefusedUpFront(t *testing.T) {
	for _, f := range []audiostreamout.SpeechFormat{{SampleRate: 11025, Channels: 1}, {SampleRate: 24000, Channels: 2}} {
		if _, err := (Synthesizer{}).StreamURL("v", f); err == nil {
			t.Fatalf("%+v accepted", f)
		}
	}
}

func TestNoKeyMeansUnavailable(t *testing.T) {
	t.Setenv("ELEVENLABS_API_KEY", "")
	if (Synthesizer{}).Available() {
		t.Fatal("available without a key")
	}
}

func TestAPlanRefusalSaysWhatToChange(t *testing.T) {
	if got := Refusal("payment_required", "v1"); got != "ElevenLabs: payment_required (voice v1 is not available on this plan; use a premade voice)" {
		t.Fatal(got)
	}
	if got := Refusal("something_new", "v1"); got != "ElevenLabs: something_new" {
		t.Fatal(got)
	}
}

func TestAProviderThatNeverAnswersIsReported(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { conn.Close() })
		}
	}()
	defer func(previous time.Duration) { ConnectTimeout = previous }(ConnectTimeout)
	ConnectTimeout = 100 * time.Millisecond

	s := Synthesizer{APIKey: testKey, URL: "ws://" + listener.Addr().String() + "/{voice_id}"}
	for _, err := range s.Synthesize(context.Background(), "hi", "", audiostreamout.DefaultSpeechFormat) {
		if !errors.Is(err, audiostreamout.ErrProviderUnavailable) || !strings.Contains(err.Error(), "did not answer") {
			t.Fatalf("err %v", err)
		}
		return
	}
	t.Fatal("no error")
}
