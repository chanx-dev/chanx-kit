package deepgram

import (
	"reflect"
	"strings"
	"testing"

	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/chanxtest"

	"go.chanx.dev/kit/kits/audiostreamin"
)

type sttConsumer struct {
	topic audiostreamin.TranscriberTopic
}

func (c sttConsumer) Topics() []chanx.TopicHandler { return []chanx.TopicHandler{c.topic} }

func joinTranscriber(t *testing.T, tr Transcriber) *chanxtest.Client {
	t.Helper()
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws", sttConsumer{tr.Topic()}, chanx.Sequential())
	c := chanxtest.Connect(t, engine, "/ws")
	c.Subscribe("transcribe:s")
	chanxtest.ReceiveUntilMessage[audiostreamin.AudioConfigMessage](c)
	return c
}

var at = chanxtest.Topic("transcribe:s")

func startAudio(c *chanxtest.Client, language *string) {
	c.Send(audiostreamin.AudioStartMessage{Payload: audiostreamin.AudioStart{
		Encoding: audiostreamin.PCM16, SampleRate: 24000, Channels: 1, Language: language,
	}}, at)
}

func TestSpeechIsTranscribedInTheNegotiatedFormat(t *testing.T) {
	address, rec := fakeDeepgram(t, listen)
	c := joinTranscriber(t, Transcriber{APIKey: testKey, URL: address})
	language := "vi"
	startAudio(c, &language)
	for i := range 5 {
		c.Send(audiostreamin.AudioChunkMessage{Payload: audiostreamin.AudioChunk{Index: i, Data: make([]byte, 4800)}}, at)
	}
	c.Send(audiostreamin.AudioEndMessage{}, at)
	final, _ := chanxtest.ReceiveUntilMessage[audiostreamin.TranscriptFinalMessage](c)
	chanxtest.ReceiveUntilMessage[audiostreamin.UtteranceEndMessage](c)

	if final.Payload.Text != "hello world" {
		t.Fatalf("final %q", final.Payload.Text)
	}
	q := rec.queries[0]
	for k, want := range map[string]string{"encoding": "linear16", "sample_rate": "24000", "channels": "1", "interim_results": "true", "language": "vi"} {
		if q.Get(k) != want {
			t.Fatalf("%s=%q, want %q", k, q.Get(k), want)
		}
	}
	rec.mu.Lock()
	audio := rec.audioBytes
	rec.mu.Unlock()
	if controls := rec.controls(); audio != 5*4800 || !reflect.DeepEqual(controls[len(controls)-2:], []string{"Finalize", "CloseStream"}) {
		t.Fatalf("%d bytes, controls %v", audio, controls)
	}
}

func TestAWrongKeyIsReported(t *testing.T) {
	address, _ := fakeDeepgram(t, listen)
	c := joinTranscriber(t, Transcriber{APIKey: "nope", URL: address})
	startAudio(c, nil)
	m, _ := chanxtest.ReceiveUntilMessage[audiostreamin.TranscriberErrorMessage](c)
	if m.Payload.Code != audiostreamin.ProviderUnavailable || !strings.Contains(m.Payload.Message, "401") {
		t.Fatalf("error %+v", m.Payload)
	}
}

func TestNoKeyIsUnavailableOnSubscribe(t *testing.T) {
	t.Setenv("DEEPGRAM_API_KEY", "")
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws", sttConsumer{Transcriber{}.Topic()}, chanx.Sequential())
	c := chanxtest.Connect(t, engine, "/ws")
	c.Subscribe("transcribe:s")
	m, _ := chanxtest.ReceiveUntilMessage[audiostreamin.TranscriberErrorMessage](c)
	if m.Payload.Code != audiostreamin.ProviderUnavailable {
		t.Fatalf("error %+v", m.Payload)
	}
}

func TestResultsMapToFrames(t *testing.T) {
	interim := `{"type":"Results","is_final":false,"start":1.0,"duration":0.5,"channel":{"alternatives":[{"transcript":"hi","confidence":0.5}]}}`
	final := strings.Replace(interim, `"is_final":false`, `"is_final":true,"speech_final":true`, 1)
	silence := strings.Replace(interim, `"hi"`, `""`, 1)

	if frames := FramesFrom([]byte(interim)); len(frames) != 1 || reflect.TypeOf(frames[0]).Name() != "PartialFrame" {
		t.Fatalf("interim %#v", frames)
	}
	frames := FramesFrom([]byte(final))
	if len(frames) != 2 || frames[0].(audiostreamin.FinalFrame).EndMs != 1500 {
		t.Fatalf("final %#v", frames)
	}
	if _, ok := frames[1].(audiostreamin.UtteranceEndFrame); !ok {
		t.Fatalf("final %#v", frames)
	}
	if frames := FramesFrom([]byte(silence)); len(frames) != 0 {
		t.Fatalf("silence %#v", frames)
	}
}
