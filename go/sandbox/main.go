// Command sandbox serves the Go kits behind the chanx-kit sandbox UI, on the framework
// of your choice:
//
//	go -C go/sandbox run . -framework gin
//	go -C go/sandbox run . notify -user chris "Build finished"
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"go.chanx.dev/layers/redis/v2"
	"go.chanx.dev/layers/v2"
	"go.chanx.dev/layers/v2/inmemory"
	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/asyncapi"

	"go.chanx.dev/kit/notification"
)

var info = asyncapi.Info{Title: "chanx-kit sandbox", Version: "0.1.0", Description: "WebSocket API exposed by the chanx-kit Go sandbox."}

func main() {
	loadEnv("../../.env")
	if len(os.Args) > 1 && os.Args[1] == "notify" {
		notify(os.Args[2:])
		return
	}
	framework := flag.String("framework", "nethttp", "nethttp, chi, gin, echo or fiber")
	addr := flag.String("addr", ":8000", "listen address")
	ui := flag.String("ui", "../../sandbox/ui/dist", "the built sandbox UI")
	flag.Parse()

	serve, ok := frameworks[*framework]
	if !ok {
		log.Fatalf("unknown framework %q", *framework)
	}
	engine := chanx.New(chanx.Config{Layers: layerSet()})
	mount(engine)
	about := map[string]any{"server": "go", "framework": *framework}
	log.Printf("chanx-kit Go sandbox on %s with %s, channel layer %s", *addr, *framework, layerName())
	log.Fatal(serve(*addr, engine, about, http.FileServer(http.Dir(*ui))))
}

// notify publishes from outside the server process; both need REDIS_URL.
func notify(args []string) {
	flags := flag.NewFlagSet("notify", flag.ExitOnError)
	user := flags.String("user", "", "address one user instead of everyone")
	_ = flags.Parse(args)
	if os.Getenv("REDIS_URL") == "" {
		log.Fatal("REDIS_URL is not set: an in-memory layer cannot reach the server process")
	}
	title := strings.Join(flags.Args(), " ")
	if title == "" {
		title = "Hello from a worker"
	}
	body := "sent from a separate process"
	payload := notification.NotificationPayload{Title: title, Body: &body}
	pub := chanx.NewPublisher(layerSet())
	ctx := context.Background()
	var err error
	if *user != "" {
		err = notification.NotifyUser(ctx, pub, *user, payload)
	} else {
		err = notification.NotifyAll(ctx, pub, payload)
	}
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Sent %q to %s\n", title, cmp(*user, "everyone"))
}

// layerSet is Redis when REDIS_URL is set, on the Python sandbox's prefix so either
// server reaches the other's clients; in memory otherwise.
func layerSet() layers.Set {
	url := os.Getenv("REDIS_URL")
	if url == "" {
		return layers.Set{"default": inmemory.New()}
	}
	l, err := redis.NewPubSub(redis.PubSubOptions{Hosts: []string{url}, Prefix: "chanx-kit"})
	if err != nil {
		log.Fatal(err)
	}
	return layers.Set{"default": l}
}

func layerName() string {
	if url := os.Getenv("REDIS_URL"); url != "" {
		return "redis (" + url + ")"
	}
	return "in-memory"
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// loadEnv reads KEY=VALUE lines the environment does not already set.
func loadEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		key, value, ok := strings.Cut(strings.TrimSpace(scanner.Text()), "=")
		if ok && !strings.HasPrefix(key, "#") && os.Getenv(key) == "" {
			_ = os.Setenv(key, value)
		}
	}
}

func cmp(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
