package rastrillo

import (
	"os"
	"path/filepath"
	"testing"
)

// A hibernate route's activator starts replicating the DB path from the
// moment the instance boots (see OpenDB's comment) — so a zero-migration
// app must still leave a file on disk for it to stream, not just an
// in-memory sql.DB handle that never touched the driver.
func TestOpenDBCreatesFileWithNoMigrations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")

	db, err := OpenDB(path, nil)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist after OpenDB, got: %v", path, err)
	}
}

// A second OpenDB against the same path (the activator's restore/wake
// cycle re-execs the binary against an already-replicated file) must
// still succeed — Ping against an existing, already-migrated database is
// not itself destructive or failure-prone.
func TestOpenDBIdempotentAgainstExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")

	db1, err := OpenDB(path, nil)
	if err != nil {
		t.Fatalf("first OpenDB: %v", err)
	}
	db1.Close()

	db2, err := OpenDB(path, nil)
	if err != nil {
		t.Fatalf("second OpenDB against existing file: %v", err)
	}
	defer db2.Close()
}
