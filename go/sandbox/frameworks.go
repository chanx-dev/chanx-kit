package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-chi/chi/v5"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/adaptor"
	"github.com/labstack/echo/v5"
	chanxecho "go.chanx.dev/adapters/echo/v2"
	chanxfiber "go.chanx.dev/adapters/fiber/v2"
	chanxgin "go.chanx.dev/adapters/gin/v2"
	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/asyncapi"
	chanxhttp "go.chanx.dev/v2/nethttp"
)

// The same consumers on each framework; only the mounting differs.
type serveFunc func(addr string, engine *chanx.Engine, about map[string]any, ui http.Handler) error

var frameworks = map[string]serveFunc{
	"nethttp": serveNetHTTP,
	"chi":     serveChi,
	"gin":     serveGin,
	"echo":    serveEcho,
	"fiber":   serveFiber,
}

func serveNetHTTP(addr string, engine *chanx.Engine, about map[string]any, ui http.Handler) error {
	mux := http.NewServeMux()
	chanxhttp.Mount(mux, engine)
	chanxhttp.MountDocs(mux, engine, "/asyncapi", info)
	mux.HandleFunc("GET /api/sandbox", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, about) })
	mux.Handle("/", ui)
	return http.ListenAndServe(addr, mux)
}

func serveChi(addr string, engine *chanx.Engine, about map[string]any, ui http.Handler) error {
	r := chi.NewRouter()
	chanxhttp.Mount(r, engine, chanxhttp.NoMethod())
	r.Get("/asyncapi", asyncapi.HTMLHandler(engine, info).ServeHTTP)
	r.Get("/asyncapi.json", asyncapi.JSONHandler(engine, info).ServeHTTP)
	r.Get("/api/sandbox", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, about) })
	r.Handle("/*", ui)
	return http.ListenAndServe(addr, r)
}

func serveGin(addr string, engine *chanx.Engine, about map[string]any, ui http.Handler) error {
	r := gin.New()
	chanxgin.Mount(r, engine)
	chanxgin.MountDocs(r, engine, "/asyncapi", info)
	r.GET("/api/sandbox", func(c *gin.Context) { c.JSON(http.StatusOK, about) })
	r.NoRoute(gin.WrapH(ui))
	return r.Run(addr)
}

func serveEcho(addr string, engine *chanx.Engine, about map[string]any, ui http.Handler) error {
	e := echo.New()
	chanxecho.Mount(e, engine)
	chanxecho.MountDocs(e, engine, "/asyncapi", info)
	e.GET("/api/sandbox", func(c *echo.Context) error { return c.JSON(http.StatusOK, about) })
	e.GET("/*", echo.WrapHandler(ui))
	return e.Start(addr)
}

func serveFiber(addr string, engine *chanx.Engine, about map[string]any, ui http.Handler) error {
	app := fiber.New()
	chanxfiber.Mount(app, engine)
	chanxfiber.MountDocs(app, engine, "/asyncapi", info)
	app.Get("/api/sandbox", func(c fiber.Ctx) error { return c.JSON(about) })
	app.Use(adaptor.HTTPHandler(ui))
	return app.Listen(addr)
}
