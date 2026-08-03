package rastrillo

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// Exactly one of Mux and Router: Serve must refuse both-set and
// neither-set before it touches a listener or a database.
func TestServeRequiresExactlyOneOfMuxAndRouter(t *testing.T) {
	both := Options{
		Mux:    http.NewServeMux(),
		Router: func(*sql.DB) (*http.ServeMux, error) { return http.NewServeMux(), nil },
	}
	if err := Serve(both); err == nil {
		t.Error("Serve accepted both Mux and Router")
	}
	if err := Serve(Options{}); err == nil {
		t.Error("Serve accepted neither Mux nor Router")
	}
}

// Pins the wiring order inside Serve itself, not just buildMux in
// isolation: the database must be open — file materialized, migrations
// applied — before Router runs, so an app can rely on the handle Router
// receives instead of racing its own open against Serve's. Returning an
// error from Router aborts Serve before it ever calls listen, so this
// never binds a socket.
func TestServeOpensTheDatabaseBeforeRouter(t *testing.T) {
	var got *sql.DB
	path := filepath.Join(t.TempDir(), "x.db")
	err := Serve(Options{DBPath: path, Router: func(d *sql.DB) (*http.ServeMux, error) {
		got = d
		return nil, errors.New("stop")
	}})
	if err == nil {
		t.Fatal("Serve ignored the Router error")
	}
	if got == nil {
		t.Error("Router ran before the database opened")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("db not materialized before Router: %v", err)
	}
}

func TestBuildMuxCallsRouterWithTheHandle(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "x.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var got *sql.DB
	mux, err := buildMux(Options{Router: func(d *sql.DB) (*http.ServeMux, error) {
		got = d
		return http.NewServeMux(), nil
	}}, db)
	if err != nil {
		t.Fatal(err)
	}
	if mux == nil {
		t.Fatal("buildMux returned a nil mux")
	}
	if got != db {
		t.Error("Router did not receive the opened handle")
	}
}

func TestBuildMuxPropagatesRouterError(t *testing.T) {
	boom := errors.New("boom")
	_, err := buildMux(Options{Router: func(*sql.DB) (*http.ServeMux, error) {
		return nil, boom
	}}, nil)
	if err == nil || !errors.Is(err, boom) {
		t.Errorf("want the Router error wrapped, got %v", err)
	}
}

func TestBuildMuxRefusesANilMuxFromRouter(t *testing.T) {
	_, err := buildMux(Options{Router: func(*sql.DB) (*http.ServeMux, error) {
		return nil, nil
	}}, nil)
	if err == nil {
		t.Error("buildMux accepted a nil mux with a nil error")
	}
}

func TestBuildMuxPassesThroughAPlainMux(t *testing.T) {
	m := http.NewServeMux()
	mux, err := buildMux(Options{Mux: m}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if mux != m {
		t.Error("buildMux did not return Options.Mux unchanged")
	}
}

// OpenDB is openDB exported: same pragmas, same eager materialization,
// same idempotent migrations. The file-exists check is the hibernate
// contract (the activator replicates the path from boot).
func TestOpenDBMaterializesAndMigrates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.db")
	db, err := OpenDB(path, []string{
		`CREATE TABLE IF NOT EXISTS t (id INTEGER PRIMARY KEY, name TEXT)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("database file not materialized at boot: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO t (name) VALUES ('a')`); err != nil {
		t.Errorf("migrated table not usable: %v", err)
	}
	db.Close()

	// Re-open: additive migrations must be idempotent.
	again, err := OpenDB(path, []string{
		`CREATE TABLE IF NOT EXISTS t (id INTEGER PRIMARY KEY, name TEXT)`,
		`ALTER TABLE t ADD COLUMN name TEXT`, // duplicate column: tolerated
	})
	if err != nil {
		t.Fatalf("re-open with idempotent migrations failed: %v", err)
	}
	again.Close()
}
