// Command blog runs the example blog.
//
// Run it from the app root — static assets are served from ./static, so
// starting the binary from anywhere else 404s both stylesheets and every
// screen renders unstyled:
//
//	cd examples/blog && go build ./cmd/blog && ./blog -addr :8080
package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"

	"github.com/carlosframework/rastrillo"

	"blog/gen"
	"blog/internal/blog"
)

func main() {
	// -socket/-addr mirror the platform's activation contract (a
	// systemd-activated listener wins over either — see rastrillo.Serve).
	socket := flag.String("socket", "", "unix socket to listen on")
	addr := flag.String("addr", "", "TCP host:port to listen on")
	dbPath := flag.String("db", "blog.db", "SQLite database file")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// The app opens its own handle: rastrillo.Serve's DBPath opens a
	// database it never hands back, and every action here reads Ctx.DB.
	// See the README's friction log, F4.
	db, err := blog.Open(*dbPath)
	if err != nil {
		logger.Error("open database", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	// A fresh Ctx per request. Actor.Human is true and Actor.Name empty:
	// honest for an app with no auth, and the one line a real deployment
	// would replace with a session lookup.
	mux := gen.Router(func(*http.Request) *rastrillo.Ctx {
		return &rastrillo.Ctx{DB: db, Logger: logger, Actor: rastrillo.Actor{Human: true}}
	})

	// The app serves its own static files — rastrillo.Serve never does.
	// "GET /static/" is a longer pattern than "GET /", so the stdlib mux
	// prefers it and no ordering care is needed.
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	if err := rastrillo.Serve(rastrillo.Options{
		Mux:    mux,
		Socket: *socket,
		Addr:   *addr,
		Logger: logger,
	}); err != nil {
		logger.Error("serve failed", "err", err)
		os.Exit(1)
	}
}
