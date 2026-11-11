package fakevoice

import (
	"testing"

	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/chanxtest"

	"go.chanx.dev/kit/kits/audiostreamout"
)

func TestOneTonePerWordAtTheRequestedRate(t *testing.T) {
	format := audiostreamout.SpeechFormat{Encoding: "pcm16", SampleRate: 16000, Channels: 1}
	pieces := WordTones("one two", format, 100, 50)
	if len(pieces) != 2 || len(pieces[0]) != (1600+800)*2 {
		t.Fatalf("%d pieces, first %d bytes", len(pieces), len(pieces[0]))
	}
}

func TestStereoDoublesTheFrames(t *testing.T) {
	mono := WordTones("hi", audiostreamout.SpeechFormat{SampleRate: 16000, Channels: 1}, 100, 50)
	stereo := WordTones("hi", audiostreamout.SpeechFormat{SampleRate: 16000, Channels: 2}, 100, 50)
	if len(stereo[0]) != 2*len(mono[0]) {
		t.Fatalf("mono %d, stereo %d", len(mono[0]), len(stereo[0]))
	}
}

type ttsConsumer struct {
	topic audiostreamout.SynthesizerTopic
}

func (c ttsConsumer) Topics() []chanx.TopicHandler { return []chanx.TopicHandler{c.topic} }

func TestTheTopicSpeaksText(t *testing.T) {
	sy := DefaultSynthesizer
	sy.Pace = 0
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws", ttsConsumer{sy.Topic()})
	c := chanxtest.Connect(t, engine, "/ws")
	c.Subscribe("speak:fake")
	c.Send(audiostreamout.SpeakMessage{Payload: audiostreamout.Speak{Text: "two words"}}, chanxtest.Topic("speak:fake"))
	end, _ := chanxtest.ReceiveUntilMessage[audiostreamout.SpeechAudioEndMessage](c)
	if end.Payload.DurationMs != 500 {
		t.Fatalf("duration %d ms, want two words of 250 ms", end.Payload.DurationMs)
	}
}
