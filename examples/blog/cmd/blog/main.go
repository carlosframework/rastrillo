// Command blog runs the example blog.
//
// Run it from the app root — static assets are served from ./static, so
// starting the binary from anywhere else 404s both stylesheets and every
// screen renders unstyled:
//
//	cd examples/blog && go build ./cmd/blog && ./blog -addr :8080
package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/carlosframework/rastrillo"

	"blog/gen"
	"blog/internal/blog"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Resolve rather than Run: this app opens its own database handle
	// (rastrillo.Serve's DBPath opens one it never hands back — see the
	// README's friction log, F4), and it needs that handle before the
	// mux exists. Resolve applies the same activation contract Run does
	// — the -socket/-addr/-db flags, a `serve` subcommand, a relative
	// path resolved inside $STATE_DIRECTORY — and hands the result back.
	opts, err := rastrillo.Resolve(rastrillo.Options{
		DBPath: "blog.db",
		Logger: logger,
	})
	if err != nil {
		logger.Error("resolve invocation", "err", err)
		os.Exit(1)
	}

	db, err := blog.Open(opts.DBPath)
	if err != nil {
		logger.Error("open database", "err", err)
		os.Exit(1)
	}
	defer db.Close()
	// The handle above is this app's own; blank DBPath so Serve doesn't
	// open a second one on the same file. Blanking it also skips Serve's
	// eager Ping, so materializing the file at boot (a hibernate route's
	// activator replicates it from boot) is now blog.Open's job — its
	// CREATE TABLE migration does it.
	opts.DBPath = ""

	// A fresh Ctx per request. Actor.Human is true and Actor.Name empty:
	// honest for an app with no auth, and the one line a real deployment
	// would replace with a session lookup.
	opts.Mux = gen.Router(func(*http.Request) *rastrillo.Ctx {
		return &rastrillo.Ctx{DB: db, Logger: logger, Actor: rastrillo.Actor{Human: true}}
	})

	// The app serves its own static files — the framework never does.
	// "GET /static/" is a longer pattern than "GET /", so the stdlib mux
	// prefers it and no ordering care is needed.
	opts.Mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	if err := rastrillo.Serve(opts); err != nil {
		logger.Error("serve failed", "err", err)
		os.Exit(1)
	}
}
