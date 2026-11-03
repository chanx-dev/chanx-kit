// Command notify sends a notification from another process. Needs the server's REDIS_URL.
//
//	REDIS_URL=redis://localhost:6399/0 go run ./cmd/notify -user chris "Hi"
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	chanx "go.chanx.dev/v2"

	"app/internal/app"
)

func main() {
	user := flag.String("user", "", "address one user instead of everyone")
	flag.Parse()
	if os.Getenv("REDIS_URL") == "" {
		log.Fatal("REDIS_URL is not set: an in-memory layer cannot reach the server. Use the same REDIS_URL as the server.")
	}
	title := strings.Join(flag.Args(), " ")
	if title == "" {
		title = "Hello from another process"
	}
	if err := app.Notify(context.Background(), chanx.NewPublisher(app.Layers()), title, *user); err != nil {
		log.Fatal(err)
	}
	to := "everyone"
	if *user != "" {
		to = *user
	}
	fmt.Printf("Sent %q to %s\n", title, to)
}
