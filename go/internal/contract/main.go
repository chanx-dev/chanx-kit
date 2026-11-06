// Command contract prints the AsyncAPI document of an app hosting one kit's contract
// topic, for scripts/contracts.py to compare with the Python kit's.
package main

import (
	"fmt"
	"os"

	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/asyncapi"

	"go.chanx.dev/kit/kits/notification"
	"go.chanx.dev/kit/kits/presence"
	"go.chanx.dev/kit/kits/roomchat"
)

var topics = map[string]chanx.TopicHandler{
	"notification": notification.BroadcastNotificationTopic{},
	"presence":     presence.PresenceTopic{},
	"room-chat":    roomchat.RoomChatTopic{},
}

type contractConsumer struct{ topic chanx.TopicHandler }

func (c contractConsumer) Topics() []chanx.TopicHandler { return []chanx.TopicHandler{c.topic} }

func main() {
	if len(os.Args) != 2 || topics[os.Args[1]] == nil {
		fmt.Fprintln(os.Stderr, "usage: contract <kit>")
		os.Exit(2)
	}
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws/contract", contractConsumer{topic: topics[os.Args[1]]}, chanx.Name("contract"))
	doc, err := asyncapi.JSON(engine, asyncapi.Info{Version: "1"})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Stdout.Write(doc)
}
