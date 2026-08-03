package devloop

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

var watched = []string{"actions", "app", "manifest"}

func TestSnapshotToleratesMissingDirs(t *testing.T) {
	root := t.TempDir() // no actions/, app/, or manifest/ at all
	snap, err := Snapshot(root, watched)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap) != 0 {
		t.Fatalf("want empty snapshot, got %v", snap)
	}
}

func TestSnapshotIgnoresUnwatchedDirs(t *testing.T) {
	root := t.TempDir()
	write(t, root, "actions/index.GET.go", "a")
	write(t, root, "gen/router.go", "generated")
	write(t, root, "main.go", "root file")
	snap, err := Snapshot(root, watched)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap) != 1 {
		t.Fatalf("want 1 entry, got %v", snap)
	}
	if _, ok := snap["actions/index.GET.go"]; !ok {
		t.Fatalf("missing actions/index.GET.go: %v", snap)
	}
}

func TestDiffDetectsAddModifyRemove(t *testing.T) {
	root := t.TempDir()
	write(t, root, "actions/index.GET.go", "one")
	write(t, root, "actions/gone.POST.go", "bye")
	prev, err := Snapshot(root, watched)
	if err != nil {
		t.Fatal(err)
	}

	write(t, root, "actions/index.GET.go", "longer content")                       // modify (size changes)
	write(t, root, "app/pages/new.go", "hi")                                       // add
	if err := os.Remove(filepath.Join(root, "actions/gone.POST.go")); err != nil { // remove
		t.Fatal(err)
	}

	next, err := Snapshot(root, watched)
	if err != nil {
		t.Fatal(err)
	}
	got := Diff(prev, next)
	want := []string{"actions/gone.POST.go", "actions/index.GET.go", "app/pages/new.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Diff = %v, want %v", got, want)
	}
}

func TestDiffSeesMtimeOnlyChange(t *testing.T) {
	// A same-size edit must still register: Stamp compares ModTime too.
	// Set mtimes explicitly — filesystem mtime granularity is too coarse
	// to race two writes in a test.
	root := t.TempDir()
	write(t, root, "actions/index.GET.go", "same size")
	full := filepath.Join(root, "actions/index.GET.go")
	base := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(full, base, base); err != nil {
		t.Fatal(err)
	}
	prev, err := Snapshot(root, watched)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(full, base.Add(time.Second), base.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	next, err := Snapshot(root, watched)
	if err != nil {
		t.Fatal(err)
	}
	if got := Diff(prev, next); len(got) != 1 || got[0] != "actions/index.GET.go" {
		t.Fatalf("Diff = %v, want [actions/index.GET.go]", got)
	}
}

func TestDiffEmptyWhenUnchanged(t *testing.T) {
	root := t.TempDir()
	write(t, root, "actions/index.GET.go", "steady")
	prev, err := Snapshot(root, watched)
	if err != nil {
		t.Fatal(err)
	}
	next, err := Snapshot(root, watched)
	if err != nil {
		t.Fatal(err)
	}
	if got := Diff(prev, next); len(got) != 0 {
		t.Fatalf("Diff = %v, want empty", got)
	}
}
