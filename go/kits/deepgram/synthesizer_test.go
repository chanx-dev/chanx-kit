package deepgram

import (
	"reflect"
	"strings"
	"testing"

	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/chanxtest"

	"go.chanx.dev/kit/kits/audiostreamout"
)

type ttsConsumer struct {
	topic audiostreamout.SynthesizerTopic
}

func (c ttsConsumer) Topics() []chanx.TopicHandler { return []chanx.TopicHandler{c.topic} }

func say(t *testing.T, s Synthesizer, session, text string) *chanxtest.Client {
	t.Helper()
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws", ttsConsumer{s.Topic()})
	c := chanxtest.Connect(t, engine, "/ws")
	c.Subscribe("speak:" + session)
	c.Send(audiostreamout.SpeakMessage{Payload: audiostreamout.Speak{Text: text}}, chanxtest.Topic("speak:"+session))
	return c
}

func TestTextIsSpokenAsWholeFrames(t *testing.T) {
	address, rec := fakeDeepgram(t, speak)
	c := say(t, Synthesizer{APIKey: testKey, URL: address}, "dg1", "hi there")
	end, _ := chanxtest.ReceiveUntilMessage[audiostreamout.SpeechAudioEndMessage](c)

	if end.Payload.DurationMs != 160 {
		t.Fatalf("duration %d ms, want 20 ms per character", end.Payload.DurationMs)
	}
	q := rec.queries[0]
	if q.Get("model") != "aura-2-thalia-en" || q.Get("encoding") != "linear16" || q.Get("sample_rate") != "24000" {
		t.Fatalf("query %v", q)
	}
	if controls := rec.controls(); !reflect.DeepEqual(controls, []string{"Speak", "Flush", "Close"}) {
		t.Fatalf("sent %v", controls)
	}
}

func TestAFormatAuraCannotMakeIsRefused(t *testing.T) {
	_, err := Synthesizer{}.SpeakURL("", audiostreamout.SpeechFormat{SampleRate: 22050, Channels: 1})
	if err == nil || !strings.Contains(err.Error(), "mono PCM") {
		t.Fatalf("err %v", err)
	}
}

func TestAWrongKeyIsReportedOnTheUtterance(t *testing.T) {
	address, _ := fakeDeepgram(t, speak)
	c := say(t, Synthesizer{APIKey: "nope", URL: address}, "dg2", "hi")
	m, _ := chanxtest.ReceiveUntilMessage[audiostreamout.SynthesizerErrorMessage](c)
	if m.Payload.Code != audiostreamout.ProviderUnavailable || !strings.Contains(m.Payload.Message, "401") {
		t.Fatalf("error %+v", m.Payload)
	}
}
