package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/carlosframework/rastrillo/ui"
)

// runNew implements `rastrillo new <name>`: go.mod, one starter action,
// a main.go wiring Serve to the (not-yet-generated) router, then runs
// generate once so `go build` works immediately (design doc §11).
//
// The starter is a plain hand-written action, not a Resource/TOML
// manifest: manifests are optional per route (design doc §3), and the
// manifest/codegen-with-skip system isn't built yet (see README.md) —
// scaffolding one would be scaffolding a feature that doesn't exist.
func runNew(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: rastrillo new <name>")
	}
	name := args[0]
	if _, err := os.Stat(name); err == nil {
		return fmt.Errorf("%s already exists", name)
	}

	dirs := []string{
		name,
		filepath.Join(name, "actions"),
		filepath.Join(name, "cmd", name),
		filepath.Join(name, "static"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}

	files := map[string]string{
		filepath.Join(name, "go.mod"):                  fmt.Sprintf(goModTemplate, name),
		filepath.Join(name, "actions", "index.GET.go"): actionTemplate,
		filepath.Join(name, "cmd", name, "main.go"):    fmt.Sprintf(mainTemplate, name),
		// The design-token stylesheet, delivered once. rastrillo.Serve
		// never serves CSS at runtime; from here on this is an ordinary
		// app-owned file that new/generate never touch again.
		filepath.Join(name, "static", "tokens.css"): string(ui.TokensCSS()),
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}

	fmt.Printf("rastrillo new: scaffolded %s/\n", name)
	fmt.Println("  go.mod")
	fmt.Println("  actions/index.GET.go")
	fmt.Printf("  cmd/%s/main.go\n", name)
	fmt.Println("  static/tokens.css")

	if err := runGenerate([]string{name}); err != nil {
		return fmt.Errorf("initial generate: %w", err)
	}
	// go build ./... discards its output whenever it matches more than one
	// package (actions, gen, gen/actions/..., cmd/<name> — see `go help
	// build`), even though cmd/<name> is the only `main` among them. Build
	// that package by its own import path so the binary actually lands here.
	fmt.Printf("\ncd %s && go build ./cmd/%[1]s\n", name)
	return nil
}

const goModTemplate = `module %s

go 1.22

require github.com/carlosframework/rastrillo v0.1.0
`

const actionTemplate = `package actions

import (
	"fmt"
	"net/http"

	"github.com/carlosframework/rastrillo"
)

func Handle(ctx *rastrillo.Ctx, w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Hello, World — this is a rastrillo app.")
}
`

const mainTemplate = `package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"

	"github.com/carlosframework/rastrillo"

	"%[1]s/gen"
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

	// The app serves its own static files — rastrillo.Serve never does.
	// static/tokens.css was scaffolded here once by rastrillo new; edit
	// it, replace it, or delete this handler if you serve assets some
	// other way.
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
`
