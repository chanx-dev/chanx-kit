// Run with go run . (API and WebSocket on :8000).
package main

import (
	"log"
	"net/http"

	"github.com/labstack/echo/v5"
	chanxecho "go.chanx.dev/adapters/echo/v2"

	"app/internal/app"
)

func main() {
	engine := app.New()
	e := echo.New()
	chanxecho.Mount(e, engine)
	chanxecho.MountDocs(e, engine, "/asyncapi", app.Info)
	e.POST("/api/notify", func(c *echo.Context) error {
		var req app.NotifyRequest
		if err := c.Bind(&req); err != nil {
			return err
		}
		if err := app.Notify(c.Request().Context(), engine.Publisher(), req.Title, req.User); err != nil {
			return err
		}
		return c.NoContent(http.StatusOK)
	})
	// Serves the built UI (npm --prefix web run build).
	e.GET("/*", echo.WrapHandler(http.FileServer(http.Dir("web/dist"))))
	log.Fatal(e.Start(":8000"))
}
