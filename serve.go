// Package rastrillo is the CARLOS web framework — the shape of a CARLOS
// app, the way carlosframework/platform is the shape of the deployment
// substrate it runs on. See the design doc for the full picture:
// https://github.com/carlosframework/platform/blob/main/docs/superpowers/specs/2026-08-01-carlos-framework-design.md
//
// This is a v1 walking skeleton: the filesystem-routing generator, the
// action signature, and this bootstrap. Manifests/codegen-with-skip, the
// crypto core, WebAuthn, agents, localization, and blob storage are
// designed in the doc above but not yet built here — see README.md.
package rastrillo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "modernc.org/sqlite"
)

// BuildVersion is set via -ldflags at build time (see cmd/rastrillo).
// The platform's deploy verification polls GET /api/version on every
// instance socket expecting exactly this — see blueprint.md, "The carlos
// core": "every instance must also serve GET /api/version reporting its
// build sha."
var BuildVersion = "dev"

// Options configures Serve.
type Options struct {
	// Mux is the app's router — normally gen/router.go's output (design
	// doc §4). Exactly one of Mux and Router must be set.
	Mux *http.ServeMux

	// Router, if set, builds the app's mux after the database opens:
	// Serve calls it with the *sql.DB opened from DBPath — pragmas,
	// eager ping, and Migrations already applied — and serves the mux
	// it returns. This is how an app puts the framework-opened handle
	// in its per-request Ctx without hand-copying the DSN (the blog's
	// friction log, F4):
	//
	//	Router: func(db *sql.DB) (*http.ServeMux, error) {
	//		return gen.Router(func(*http.Request) *rastrillo.Ctx {
	//			return &rastrillo.Ctx{DB: db, Logger: logger}
	//		}), nil
	//	},
	//
	// Exactly one of Mux and Router must be set. With DBPath empty,
	// Router is called with a nil db — an app without a database can
	// still defer its mux construction. Serve owns the handle and
	// closes it when Serve returns; do not retain it past that. An app
	// that needs a handle outside Serve's lifetime calls OpenDB itself.
	Router func(db *sql.DB) (*http.ServeMux, error)

	// Wrap, if set, wraps the app's mux — the one seam for app
	// middleware: sessions, CSRF, panic pages, authorization
	// (gleester's friction, James 2026-08-04). It runs inside the
	// framework's chrome: GET /healthz and GET /api/version are
	// answered outside it (platform probes never traverse app
	// middleware), and locale-prefix stripping happens before it,
	// so middleware sees the same paths routes match on. Nil means
	// no wrapping. Returning nil is a boot error.
	Wrap func(http.Handler) http.Handler

	// DBPath, if set, opens a SQLite database with the pragma ordering
	// and connection settings the survey found hand-propagated,
	// error-prone, repo to repo (design doc §5): busy_timeout set
	// *before* journal_mode=WAL, then SetMaxOpenConns(1).
	DBPath string

	// Migrations are applied in order at boot, idempotently: each must
	// be safe to run against a database that already has it applied
	// (CREATE TABLE IF NOT EXISTS, or an ALTER whose "duplicate column"
	// error is ignored) — additive-only, per the family's hard-won rule.
	Migrations []string

	// Socket and Addr mirror the platform's activation contract (see
	// testdata/echoapp in carlosframework/platform): a unix socket path,
	// or a TCP host:port for local dev. If both are empty, Serve checks
	// for a systemd-activated listener (LISTEN_FDS) before falling back
	// to Addr ":8080".
	Socket string
	Addr   string

	// Locales declares the app's locale codes (design doc §10) — the
	// catalogs LocaleFS carries as locales/<code>.toml. Empty means a
	// monolingual app: no locale middleware is installed and requests
	// pay nothing.
	Locales []string

	// DefaultLocale is the locale for unprefixed requests that match
	// nothing else, and the first fallback layer for missing keys.
	// Empty defaults to Locales[0].
	DefaultLocale string

	// LocaleFS provides the locales/<code>.toml catalog files —
	// normally an embed.FS rooted at the app directory. Nil is legal:
	// lookups fall back to the key itself, which keeps a missing
	// catalog visible instead of silently blank (§10).
	LocaleFS fs.FS

	// BaseCatalog optionally supplies a base catalog that sits UNDER
	// every app catalog (Locales' own doc comment: requested locale's
	// app catalog, then the default locale's app catalog, then this) —
	// normally the generated gen/locales/locales.go var BaseCatalog a
	// manifest resource's field labels and shared ui.* chrome strings
	// compile to (design doc §9's manifest system; internal/generate's
	// EmitLocales emits it from the same map as the human-readable
	// gen/locales/en.toml, so the two cannot drift). Nil is legal — an
	// app with no manifest resources has nothing to layer.
	BaseCatalog Catalog

	// Logger defaults to slog.Default() if nil.
	Logger *slog.Logger
}

// Serve opens the database (if configured), applies migrations, resolves
// the platform's activation contract for a listener, and serves until the
// process receives SIGTERM/SIGINT. It always answers GET /healthz itself
// — the manifest/action layer never has to remember to.
func Serve(opts Options) error {
	if (opts.Mux == nil) == (opts.Router == nil) {
		return errors.New("rastrillo: exactly one of Options.Mux and Options.Router must be set")
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	var db *sql.DB
	var err error
	if opts.DBPath != "" {
		db, err = OpenDB(opts.DBPath, opts.Migrations)
		if err != nil {
			return fmt.Errorf("rastrillo: open database: %w", err)
		}
		defer db.Close()
	}

	opts.Mux, err = buildMux(opts, db)
	if err != nil {
		return err
	}

	handler, err := buildHandler(opts)
	if err != nil {
		// buildHandler's error sources (NewLocales, the Wrap nil-handler
		// check) already carry the rastrillo: prefix, so no re-wrap here.
		return err
	}

	ln, err := listen(opts.Socket, opts.Addr)
	if err != nil {
		return fmt.Errorf("rastrillo: listen: %w", err)
	}
	logger.Info("rastrillo: serving", "addr", ln.Addr().String(), "version", BuildVersion)

	srv := &http.Server{Handler: handler}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-sigCh:
		logger.Info("rastrillo: shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			return err
		}
	}
	return nil
}

// buildHandler assembles the serving handler: the framework's own
// endpoints, the app mux (wrapped by Options.Wrap when set), and
// — when Options.Locales is set — the locale middleware wrapped around
// the whole thing, so a locale prefix strips before routing and the
// translator rides the request context (§10). Split from Serve so the
// assembly is testable without sockets.
func buildHandler(opts Options) (http.Handler, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "ok")
	})
	mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, BuildVersion)
	})
	app := http.Handler(opts.Mux)
	if opts.Wrap != nil {
		if app = opts.Wrap(opts.Mux); app == nil {
			return nil, errors.New("rastrillo: Options.Wrap returned a nil handler")
		}
	}
	mux.Handle("/", app)

	if len(opts.Locales) == 0 {
		return mux, nil
	}
	def := opts.DefaultLocale
	if def == "" {
		def = opts.Locales[0]
	}
	loc, err := NewLocales(opts.Locales, def, opts.BaseCatalog, opts.LocaleFS)
	if err != nil {
		return nil, err
	}
	return loc.Middleware(mux), nil
}

// buildMux resolves the Mux/Router choice. Router runs after the
// database opens so the app can close over the framework-opened handle
// — the entire point of the seam (F4).
func buildMux(opts Options, db *sql.DB) (*http.ServeMux, error) {
	if opts.Router == nil {
		return opts.Mux, nil
	}
	mux, err := opts.Router(db)
	if err != nil {
		return nil, fmt.Errorf("rastrillo: build router: %w", err)
	}
	if mux == nil {
		return nil, errors.New("rastrillo: Options.Router returned a nil mux")
	}
	return mux, nil
}

// listen resolves the platform's activation contract, exactly matching
// carlosframework/platform's testdata/echoapp: a systemd-activated
// listener (LISTEN_FDS=1, fd 3 — the carlos-app@.socket contract) takes
// priority; otherwise Socket (unix) or Addr (TCP).
func listen(socket, addr string) (net.Listener, error) {
	if n, _ := strconv.Atoi(os.Getenv("LISTEN_FDS")); n >= 1 {
		if pid := os.Getenv("LISTEN_PID"); pid == "" || pid == strconv.Itoa(os.Getpid()) {
			return net.FileListener(os.NewFile(3, "listen-fd"))
		}
	}
	if socket != "" {
		os.Remove(socket)
		return net.Listen("unix", socket)
	}
	if addr == "" {
		addr = ":8080"
	}
	return net.Listen("tcp", addr)
}

// OpenDB applies the SQLite convention the survey found hand-propagated,
// with fixes, repo to repo (design doc §5): busy_timeout set *before*
// journal_mode=WAL — the reverse order crashes with SQLITE_BUSY under
// concurrent open, titogo's real fix — then SetMaxOpenConns(1), then an
// eager ping so the file exists on disk from boot, then migrate.
//
// Exported so tests and non-Serve contexts get the corrected opener
// instead of reproducing the DSN by hand (the blog's F4).
func OpenDB(path string, migrations []string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	// database/sql's Open is lazy — it never touches the driver, so with
	// zero migrations the file would never materialize. A hibernate
	// route's activator starts replicating this path from boot, so a
	// zero-migration app must still create it: Ping forces the
	// connection open now, at boot, instead of on the first request.
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping %s: %w", path, err)
	}

	for i, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			if isDuplicateColumn(err) {
				continue
			}
			db.Close()
			return nil, fmt.Errorf("migration %d: %w", i, err)
		}
	}
	return db, nil
}

// isDuplicateColumn matches the one class of migration error the family
// convention treats as success: an additive ALTER TABLE ... ADD COLUMN
// re-run against a database that already has it.
func isDuplicateColumn(err error) bool {
	return strings.Contains(err.Error(), "duplicate column")
}
