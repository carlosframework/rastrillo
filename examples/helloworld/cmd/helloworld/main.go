package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"

	"github.com/carlosframework/rastrillo"

	"helloworld/gen"
)

func main() {
	// -socket/-addr mirror the platform's activation contract (a
	// systemd-activated listener wins over either — see rastrillo.Serve).
	socket := flag.String("socket", "", "unix socket to listen on")
	addr := flag.String("addr", "", "TCP host:port to listen on")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// A single shared Ctx for now: this app has no per-request state
	// yet (no DB, no locale, no scope). Once it does, build a fresh
	// Ctx per request here instead.
	ctx := &rastrillo.Ctx{Logger: logger}
	mux := gen.Router(func(*http.Request) *rastrillo.Ctx { return ctx })

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
