// Command contract prints the AsyncAPI document of an app hosting one kit's contract
// topic, for scripts/contracts.py to compare with the Python kit's.
package main

import (
	"fmt"
	"os"

	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/asyncapi"

	"go.chanx.dev/kit/kits/agui"
	"go.chanx.dev/kit/kits/audiostreamin"
	"go.chanx.dev/kit/kits/audiostreamout"
	"go.chanx.dev/kit/kits/deepgram"
	"go.chanx.dev/kit/kits/elevenlabs"
	"go.chanx.dev/kit/kits/fakevoice"
	"go.chanx.dev/kit/kits/notification"
	"go.chanx.dev/kit/kits/openai"
	"go.chanx.dev/kit/kits/presence"
	"go.chanx.dev/kit/kits/roomchat"
)

var topics = map[string]chanx.TopicHandler{
	"ag-ui":                    agui.AgUiTopic{},
	"audio-stream-in":          audiostreamin.TranscriberTopic{},
	"audio-stream-out":         audiostreamout.SynthesizerTopic{},
	"fake-voice transcriber@1": fakevoice.DefaultTranscriber.Topic(),
	"fake-voice synthesizer@1": fakevoice.DefaultSynthesizer.Topic(),
	"deepgram transcriber@1":   deepgram.Transcriber{}.Topic(),
	"deepgram synthesizer@1":   deepgram.Synthesizer{}.Topic(),
	"elevenlabs transcriber@1": elevenlabs.Transcriber{}.Topic(),
	"elevenlabs synthesizer@1": elevenlabs.Synthesizer{}.Topic(),
	"openai transcriber@1":     openai.Transcriber{}.Topic(),
	"openai synthesizer@1":     openai.Synthesizer{}.Topic(),
	"notification":             notification.BroadcastNotificationTopic{},
	"presence":                 presence.PresenceTopic{},
	"room-chat":                roomchat.RoomChatTopic{},
}

type contractConsumer struct{ topic chanx.TopicHandler }

func (c contractConsumer) Topics() []chanx.TopicHandler { return []chanx.TopicHandler{c.topic} }

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: contract <kit> [contract]")
		os.Exit(2)
	}
	// A provider implementing several contracts is listed once per contract.
	topic := topics[os.Args[1]]
	if len(os.Args) > 2 && topics[os.Args[1]+" "+os.Args[2]] != nil {
		topic = topics[os.Args[1]+" "+os.Args[2]]
	}
	if topic == nil {
		fmt.Fprintln(os.Stderr, "unknown kit:", os.Args[1])
		os.Exit(2)
	}
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws/contract", contractConsumer{topic: topic}, chanx.Name("contract"))
	doc, err := asyncapi.JSON(engine, asyncapi.Info{Version: "1"})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Stdout.Write(doc)
}
