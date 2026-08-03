package blogtest

import (
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carlosframework/rastrillo"

	"blog/gen"
	"blog/internal/blog"
)

// newApp builds a whole app per test: a fresh file-backed SQLite database
// (a file, not :memory:, because SetMaxOpenConns(1) plus WAL is the
// configuration being exercised) and the real generated router, wired
// exactly as cmd/blog/main.go wires it.
func newApp(t *testing.T) (http.Handler, *sql.DB) {
	t.Helper()
	db, err := blog.Open(filepath.Join(t.TempDir(), "blog.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux := gen.Router(func(*http.Request) *rastrillo.Ctx {
		return &rastrillo.Ctx{DB: db, Logger: logger, Actor: rastrillo.Actor{Human: true}}
	})
	return mux, db
}

func get(t *testing.T, h http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func post(t *testing.T, h http.Handler, target string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// seed creates a post through the store, so the tests exercise it too.
func seed(t *testing.T, db *sql.DB, title, body string, published bool) int64 {
	t.Helper()
	id, err := blog.Create(db, title, body)
	if err != nil {
		t.Fatalf("seed %q: %v", title, err)
	}
	if published {
		if err := blog.SetPublished(db, id, true); err != nil {
			t.Fatalf("publish %q: %v", title, err)
		}
	}
	return id
}
