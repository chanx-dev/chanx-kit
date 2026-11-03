// Run with go run . (API and WebSocket on :8000).
package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	chanxgin "go.chanx.dev/adapters/gin/v2"

	"app/internal/app"
)

func main() {
	engine := app.New()
	r := gin.Default()
	chanxgin.Mount(r, engine)
	chanxgin.MountDocs(r, engine, "/asyncapi", app.Info)
	r.POST("/api/notify", func(c *gin.Context) {
		var req app.NotifyRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
			return
		}
		if err := app.Notify(c, engine.Publisher(), req.Title, req.User); err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"detail": err.Error()})
			return
		}
		c.Status(http.StatusOK)
	})
	// Serves the built UI (npm --prefix web run build).
	r.NoRoute(gin.WrapH(http.FileServer(http.Dir("web/dist"))))
	log.Fatal(r.Run(":8000"))
}
