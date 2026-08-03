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
	// doc §4). Required.
	Mux *http.ServeMux

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

	// Logger defaults to slog.Default() if nil.
	Logger *slog.Logger
}

// Serve opens the database (if configured), applies migrations, resolves
// the platform's activation contract for a listener, and serves until the
// process receives SIGTERM/SIGINT. It always answers GET /healthz itself
// — the manifest/action layer never has to remember to.
func Serve(opts Options) error {
	if opts.Mux == nil {
		return errors.New("rastrillo: Options.Mux is required")
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	var db *sql.DB
	if opts.DBPath != "" {
		var err error
		db, err = openDB(opts.DBPath, opts.Migrations)
		if err != nil {
			return fmt.Errorf("rastrillo: open database: %w", err)
		}
		defer db.Close()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "ok")
	})
	mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, BuildVersion)
	})
	mux.Handle("/", opts.Mux)

	ln, err := listen(opts.Socket, opts.Addr)
	if err != nil {
		return fmt.Errorf("rastrillo: listen: %w", err)
	}
	logger.Info("rastrillo: serving", "addr", ln.Addr().String(), "version", BuildVersion)

	srv := &http.Server{Handler: mux}
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

// openDB applies the SQLite convention the survey found hand-propagated,
// with fixes, repo to repo (design doc §5): busy_timeout set *before*
// journal_mode=WAL — the reverse order crashes with SQLITE_BUSY under
// concurrent open, titogo's real fix — then SetMaxOpenConns(1), then
// migrate.
func openDB(path string, migrations []string) (*sql.DB, error) {
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
		return nil, fmt.Errorf("ping: %w", err)
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
