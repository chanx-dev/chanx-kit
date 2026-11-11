package fakevoice

import (
	"context"
	"encoding/binary"
	"reflect"
	"testing"

	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/chanxtest"

	"go.chanx.dev/kit/kits/audiostreamin"
)

// loud is ms of audio loud enough to count as speech, at 48 bytes per ms.
func loud(ms int) []byte {
	b := make([]byte, ms*48)
	for i := 0; i < len(b); i += 2 {
		binary.LittleEndian.PutUint16(b[i:], 4000)
	}
	return b
}

func quiet(ms int) []byte { return make([]byte, ms*48) }

func described(t *testing.T, script []string, wordsPerFinal int, chunks ...[]byte) []string {
	t.Helper()
	tr := Transcriber{Script: script, MsPerWord: 100, WordsPerFinal: wordsPerFinal, SpeechLevel: 500, SilenceMs: 300}
	s, _ := tr.Open(context.Background(), audiostreamin.AudioStart{SampleRate: 24000, Channels: 1})
	ctx := context.Background()
	for _, c := range chunks {
		_ = s.Send(ctx, c)
	}
	_ = s.Finish(ctx)
	var out []string
	for f := range s.Frames(ctx) {
		switch f := f.(type) {
		case audiostreamin.SpeechStartedFrame:
			out = append(out, "started")
		case audiostreamin.PartialFrame:
			out = append(out, "partial:"+f.Text)
		case audiostreamin.FinalFrame:
			out = append(out, "final:"+f.Text)
		case audiostreamin.UtteranceEndFrame:
			out = append(out, "end")
		}
	}
	return out
}

func TestWordsArePacedByLoudAudio(t *testing.T) {
	got := described(t, []string{"a", "b", "c"}, 10, loud(250))
	want := []string{"started", "partial:a", "partial:a b", "final:a b", "end"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestLongSpeechIsSplitIntoFinalsAndTheScriptRepeats(t *testing.T) {
	var finals []string
	for _, d := range described(t, []string{"a", "b"}, 2, loud(500)) {
		if len(d) > 6 && d[:6] == "final:" {
			finals = append(finals, d[6:])
		}
	}
	if !reflect.DeepEqual(finals, []string{"a b", "a b", "a"}) {
		t.Fatalf("finals %v", finals)
	}
}

func TestAPauseEndsTheUtteranceAndSpeechStartsAnother(t *testing.T) {
	got := described(t, []string{"one", "two", "three", "four"}, 10,
		loud(100), loud(100), quiet(100), quiet(100), quiet(100), quiet(100), loud(100), loud(100))
	want := []string{
		"started", "partial:one", "partial:one two", "final:one two", "end",
		"started", "partial:three", "partial:three four", "final:three four", "end",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestAShortPauseDoesNotEndTheUtterance(t *testing.T) {
	ends := 0
	for _, d := range described(t, []string{"a", "b", "c"}, 10, loud(100), quiet(200), loud(100)) {
		if d == "end" {
			ends++
		}
	}
	if ends != 1 {
		t.Fatalf("%d utterance ends", ends)
	}
}

func TestSilenceAloneProducesNothing(t *testing.T) {
	if got := described(t, []string{"a"}, 10, quiet(1000)); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

type sttConsumer struct {
	topic audiostreamin.TranscriberTopic
}

func (c sttConsumer) Topics() []chanx.TopicHandler { return []chanx.TopicHandler{c.topic} }

// The topic transcribes a second of speech as the script, end to end.
func TestTheTopicTranscribesSpeech(t *testing.T) {
	tr := DefaultTranscriber
	tr.Script, tr.MsPerWord, tr.WordsPerFinal = []string{"hello", "world"}, 500, 2
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws", sttConsumer{tr.Topic()}, chanx.Sequential())
	c := chanxtest.Connect(t, engine, "/ws")
	c.Subscribe("transcribe:s")
	chanxtest.ReceiveUntilMessage[audiostreamin.AudioConfigMessage](c)

	at := chanxtest.Topic("transcribe:s")
	c.Send(audiostreamin.AudioStartMessage{Payload: audiostreamin.AudioStart{Encoding: audiostreamin.PCM16, SampleRate: 24000, Channels: 1}}, at)
	for i := range 10 {
		c.Send(audiostreamin.AudioChunkMessage{Payload: audiostreamin.AudioChunk{Index: i, Data: loud(100)}}, at)
	}
	c.Send(audiostreamin.AudioEndMessage{}, at)
	final, _ := chanxtest.ReceiveUntilMessage[audiostreamin.TranscriptFinalMessage](c)
	if final.Payload.Text != "hello world" {
		t.Fatalf("final %q", final.Payload.Text)
	}
	chanxtest.ReceiveUntilMessage[audiostreamin.UtteranceEndMessage](c)
}
