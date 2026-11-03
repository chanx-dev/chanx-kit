// Run with go run . (API and WebSocket on :8000).
package main

import (
	"log"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/static"
	chanxfiber "go.chanx.dev/adapters/fiber/v2"

	"app/internal/app"
)

func main() {
	engine := app.New()
	server := fiber.New()
	chanxfiber.Mount(server, engine)
	chanxfiber.MountDocs(server, engine, "/asyncapi", app.Info)
	server.Post("/api/notify", func(c fiber.Ctx) error {
		var req app.NotifyRequest
		if err := c.Bind().JSON(&req); err != nil {
			return err
		}
		return app.Notify(c.Context(), engine.Publisher(), req.Title, req.User)
	})
	// Serves the built UI (npm --prefix web run build).
	server.Use("/", static.New("web/dist"))
	log.Fatal(server.Listen(":8000"))
}
