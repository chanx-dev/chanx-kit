// Run with go run . (API and WebSocket on :8000).
package main

import (
	"encoding/json"
	"log"
	"net/http"

	chanxhttp "go.chanx.dev/v2/nethttp"

	"app/internal/app"
)

func main() {
	engine := app.New()
	mux := http.NewServeMux()
	chanxhttp.Mount(mux, engine)
	chanxhttp.MountDocs(mux, engine, "/asyncapi", app.Info)
	mux.HandleFunc("POST /api/notify", func(w http.ResponseWriter, r *http.Request) {
		var req app.NotifyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := app.Notify(r.Context(), engine.Publisher(), req.Title, req.User); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	// Serves the built UI (npm --prefix web run build).
	mux.Handle("/", http.FileServer(http.Dir("web/dist")))
	log.Fatal(http.ListenAndServe(":8000", mux))
}
