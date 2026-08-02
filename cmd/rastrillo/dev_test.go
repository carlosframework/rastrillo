package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindAppPkg(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "cmd", "helloworld"), 0o755); err != nil {
		t.Fatal(err)
	}
	pkg, name, err := findAppPkg(dir)
	if err != nil {
		t.Fatal(err)
	}
	if pkg != "./cmd/helloworld" || name != "helloworld" {
		t.Fatalf("got (%q, %q), want (./cmd/helloworld, helloworld)", pkg, name)
	}
}

func TestFindAppPkgNoCmdDir(t *testing.T) {
	_, _, err := findAppPkg(t.TempDir())
	if err == nil {
		t.Fatal("want error for missing cmd/, got nil")
	}
	if !strings.Contains(err.Error(), "cmd/") {
		t.Fatalf("error should mention cmd/: %v", err)
	}
}

func TestFindAppPkgAmbiguous(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"one", "two"} {
		if err := os.MkdirAll(filepath.Join(dir, "cmd", n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	_, _, err := findAppPkg(dir)
	if err == nil {
		t.Fatal("want error for ambiguous cmd/, got nil")
	}
}

func TestFindAppPkgIgnoresFiles(t *testing.T) {
	// A stray file under cmd/ (README, .DS_Store) must not count.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "cmd", "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cmd", "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	pkg, name, err := findAppPkg(dir)
	if err != nil {
		t.Fatal(err)
	}
	if pkg != "./cmd/app" || name != "app" {
		t.Fatalf("got (%q, %q), want (./cmd/app, app)", pkg, name)
	}
}

func TestSplitAppArgs(t *testing.T) {
	own, app := splitAppArgs([]string{"mydir", "--", "-addr", ":9000"})
	if len(own) != 1 || own[0] != "mydir" {
		t.Fatalf("own = %v, want [mydir]", own)
	}
	if len(app) != 2 || app[0] != "-addr" || app[1] != ":9000" {
		t.Fatalf("app = %v, want [-addr :9000]", app)
	}

	own, app = splitAppArgs([]string{"--", "-addr", ":9000"})
	if len(own) != 0 || len(app) != 2 {
		t.Fatalf("got (%v, %v), want ([], [-addr :9000])", own, app)
	}

	own, app = splitAppArgs(nil)
	if len(own) != 0 || len(app) != 0 {
		t.Fatalf("got (%v, %v), want ([], [])", own, app)
	}
}
