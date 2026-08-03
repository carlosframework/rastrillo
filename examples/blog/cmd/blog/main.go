// Command blog runs the example blog.
//
// Run it from the app root — static assets are served from ./static, so
// starting the binary from anywhere else 404s both stylesheets and every
// screen renders unstyled:
//
//	cd examples/blog && go build ./cmd/blog && ./blog -addr :8080
package main

import (
	"database/sql"
	"log/slog"
	"net/http"
	"os"

	"github.com/carlosframework/rastrillo"

	blogassets "blog"
	"blog/gen"
	"blog/internal/blog"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Run end to end: rastrillo resolves the activation argv, opens the
	// database (pragmas, eager ping, the schema migration), and hands
	// the *sql.DB back through Router — the F4 seam. No hand-copied
	// DSN, no Resolve dance, no double-open to avoid.
	err := rastrillo.Run(rastrillo.Options{
		DBPath:     "blog.db",
		Migrations: []string{blog.Migration},
		Logger:     logger,
		Router: func(db *sql.DB) (*http.ServeMux, error) {
			// A fresh Ctx per request. Actor.Human is true and
			// Actor.Name empty: honest for an app with no auth, and
			// the one line a real deployment would replace with a
			// session lookup.
			mux := gen.Router(func(*http.Request) *rastrillo.Ctx {
				return &rastrillo.Ctx{DB: db, Logger: logger, Actor: rastrillo.Actor{Human: true}}
			})

			// The app serves its own static files — the framework
			// never does. They are embedded (see assets.go), so the
			// binary is self-contained wherever it starts (F8).
			// "GET /static/" is a longer pattern than "GET /{$}", so
			// the stdlib mux prefers it and no ordering care is
			// needed.
			mux.Handle("GET /static/", http.FileServerFS(blogassets.StaticFS))
			return mux, nil
		},
	})
	if err != nil {
		logger.Error("serve failed", "err", err)
		os.Exit(1)
	}
}
