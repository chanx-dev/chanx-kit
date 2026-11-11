package elevenlabs

import (
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

var at = chanxtest.Topic("transcribe:s")

func speakInto(t *testing.T, tr Transcriber, language *string) *chanxtest.Client {
	t.Helper()
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws", sttConsumer{tr.Topic()}, chanx.Sequential())
	c := chanxtest.Connect(t, engine, "/ws")
	c.Subscribe("transcribe:s")
	chanxtest.ReceiveUntilMessage[audiostreamin.AudioConfigMessage](c)
	c.Send(audiostreamin.AudioStartMessage{Payload: audiostreamin.AudioStart{
		Encoding: audiostreamin.PCM16, SampleRate: 24000, Channels: 1, Language: language,
	}}, at)
	for i := range 5 {
		c.Send(audiostreamin.AudioChunkMessage{Payload: audiostreamin.AudioChunk{Index: i, Data: make([]byte, 4800)}}, at)
	}
	return c
}

func TestTheFormatAndLanguageReachScribe(t *testing.T) {
	address, rec := fakeElevenLabs(t, scribe)
	language := "vi"
	c := speakInto(t, Transcriber{APIKey: testKey, URL: address}, &language)
	c.Send(audiostreamin.AudioEndMessage{}, at)
	final, _ := chanxtest.ReceiveUntilMessage[audiostreamin.TranscriptFinalMessage](c)
	chanxtest.ReceiveUntilMessage[audiostreamin.UtteranceEndMessage](c)

	if final.Payload.Text != "hello world" {
		t.Fatalf("final %q", final.Payload.Text)
	}
	q := rec.queries[0]
	for k, want := range map[string]string{"audio_format": "pcm_24000", "commit_strategy": "vad", "language_code": "vi", "model_id": "scribe_v2_realtime"} {
		if q.Get(k) != want {
			t.Fatalf("%s=%q, want %q", k, q.Get(k), want)
		}
	}
	rec.lock(func() {
		if rec.audioBytes != 5*4800 || rec.commits != 1 {
			t.Fatalf("%d bytes, %d commits", rec.audioBytes, rec.commits)
		}
	})
}

func TestTheFirstPartialOfAnUtteranceReportsSpeech(t *testing.T) {
	address, _ := fakeElevenLabs(t, scribe)
	c := speakInto(t, Transcriber{APIKey: testKey, URL: address}, nil)
	started, _ := chanxtest.ReceiveUntilMessage[audiostreamin.SpeechStartedMessage](c)
	if started.Payload.AtMs != 0 {
		t.Fatalf("started at %d", started.Payload.AtMs)
	}
	chanxtest.ReceiveUntilMessage[audiostreamin.TranscriptPartialMessage](c)
}

func TestAnUnsupportedFormatIsRefused(t *testing.T) {
	_, err := Transcriber{}.RealtimeURL(audiostreamin.AudioStart{SampleRate: 11025})
	if err == nil || !strings.Contains(err.Error(), "mono PCM") {
		t.Fatalf("err %v", err)
	}
}
