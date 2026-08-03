package main

import (
	"io"
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

func TestParseDevArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantDir    string
		wantApp    []string
		wantHelp   bool
		wantErrSub string // non-empty: err must be non-nil and contain this
	}{
		{name: "no args defaults to .", args: nil, wantDir: "."},
		{name: "dir only", args: []string{"myapp"}, wantDir: "myapp"},
		{
			name:    "dir and app args",
			args:    []string{"myapp", "--", "-addr", ":9000"},
			wantDir: "myapp",
			wantApp: []string{"-addr", ":9000"},
		},
		{
			name:    "app args only, no dir",
			args:    []string{"--", "-addr", ":9000"},
			wantDir: ".",
			wantApp: []string{"-addr", ":9000"},
		},
		{
			name:       "leading dash rejected",
			args:       []string{"-addr", ":9000"},
			wantErrSub: `pass app flags after "--"`,
		},
		{name: "help -h", args: []string{"-h"}, wantHelp: true},
		{name: "help -help", args: []string{"-help"}, wantHelp: true},
		{name: "help --help", args: []string{"--help"}, wantHelp: true},
		{
			name:       "extra arg after dir rejected",
			args:       []string{".", "-addr", ":9000"},
			wantErrSub: `unexpected argument "-addr" after app directory`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, appArgs, help, err := parseDevArgs(tt.args)
			if tt.wantErrSub != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if help != tt.wantHelp {
				t.Fatalf("help = %v, want %v", help, tt.wantHelp)
			}
			if tt.wantHelp {
				return
			}
			if dir != tt.wantDir {
				t.Fatalf("dir = %q, want %q", dir, tt.wantDir)
			}
			if len(appArgs) != len(tt.wantApp) {
				t.Fatalf("appArgs = %v, want %v", appArgs, tt.wantApp)
			}
			for i := range appArgs {
				if appArgs[i] != tt.wantApp[i] {
					t.Fatalf("appArgs = %v, want %v", appArgs, tt.wantApp)
				}
			}
		})
	}
}

func TestDevUsage(t *testing.T) {
	// devUsage must go to stdout, not stderr — a help request is not an
	// error.
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	devUsage()
	w.Close()
	os.Stdout = old

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if !strings.Contains(got, "rastrillo dev [dir]") {
		t.Fatalf("usage should show the command form, got: %s", got)
	}
	if !strings.Contains(got, `"--"`) {
		t.Fatalf("usage should explain --, got: %s", got)
	}
}
