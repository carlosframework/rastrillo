package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/carlosframework/rastrillo"

	"helloworld/gen"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// A single shared Ctx for now: this app has no per-request state
	// yet (no DB, no locale, no scope). Once it does, build a fresh
	// Ctx per request here instead.
	ctx := &rastrillo.Ctx{Logger: logger}
	mux := gen.Router(func(*http.Request) *rastrillo.Ctx { return ctx })

	// Run speaks the platform's activation contract: -socket/-addr/-db
	// flags for agent exec children, or a bare "serve" subcommand for
	// carlos-app@ unit tenants (see rastrillo.Run).
	if err := rastrillo.Run(rastrillo.Options{
		Mux:    mux,
		Logger: logger,
	}); err != nil {
		logger.Error("serve failed", "err", err)
		os.Exit(1)
	}
}
