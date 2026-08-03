package rastrillo

import (
	"os"
	"strings"
	"testing"
)

// The argv shapes here are the platform's, not ours: the exec-child line
// is what internal/activator/backend_exec.go spawns, the bare `serve` is
// carlos-app@.service's ExecStart, and STATE_DIRECTORY is what systemd
// sets for a unit tenant whose cwd is not its state dir.
func TestResolveInvocation(t *testing.T) {
	tests := []struct {
		name       string
		base       string // Options.DBPath before the argv is applied
		args       []string
		stateDir   string
		wantSocket string
		wantAddr   string
		wantDB     string
	}{
		{
			name:       "exec child argv",
			args:       []string{"--socket", "/run/carlos/x.sock", "--db", "/data/x.db"},
			wantSocket: "/run/carlos/x.sock",
			wantDB:     "/data/x.db",
		},
		{
			name:       "single-dash spelling",
			args:       []string{"-socket", "/run/carlos/x.sock", "-db", "/data/x.db"},
			wantSocket: "/run/carlos/x.sock",
			wantDB:     "/data/x.db",
		},
		{
			name:     "unit tenant, relative db",
			base:     "app.db",
			args:     []string{"serve"},
			stateDir: "/var/lib/carlos-app/x",
			wantDB:   "/var/lib/carlos-app/x/app.db",
		},
		{
			name:     "unit tenant, no db",
			args:     []string{"serve"},
			stateDir: "/var/lib/carlos-app/x",
			wantDB:   "",
		},
		{
			name:   "dev bare invocation",
			base:   "app.db",
			wantDB: "app.db",
		},
		{
			name:     "absolute db + stateDir",
			base:     "/data/x.db",
			stateDir: "/var/lib/carlos-app/x",
			wantDB:   "/data/x.db",
		},
		{
			name:     "-db wins over Options",
			base:     "app.db",
			args:     []string{"--db", "/data/y.db"},
			stateDir: "/var/lib/carlos-app/x",
			wantDB:   "/data/y.db",
		},
		{
			name:     "flags after serve",
			args:     []string{"serve", "-addr", ":9000"},
			wantAddr: ":9000",
		},
		{
			name:     "addr override",
			args:     []string{"-addr", ":9000"},
			wantAddr: ":9000",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveInvocation(Options{DBPath: tt.base}, tt.args, tt.stateDir)
			if err != nil {
				t.Fatalf("resolveInvocation: %v", err)
			}
			if got.Socket != tt.wantSocket {
				t.Errorf("Socket = %q, want %q", got.Socket, tt.wantSocket)
			}
			if got.Addr != tt.wantAddr {
				t.Errorf("Addr = %q, want %q", got.Addr, tt.wantAddr)
			}
			if got.DBPath != tt.wantDB {
				t.Errorf("DBPath = %q, want %q", got.DBPath, tt.wantDB)
			}
		})
	}
}

// A flag the platform never passes must fail loudly rather than be
// swallowed: backend_exec.go's control-socket comment records that
// pre-existing binaries exit 2 on unknown flags, and the platform sizes
// its rollouts around that.
func TestResolveInvocationUnknownFlag(t *testing.T) {
	if _, err := resolveInvocation(Options{}, []string{"-control-socket", "/x"}, ""); err == nil {
		t.Fatal("resolveInvocation: want error for unknown flag, got nil")
	}
}

func TestResolveInvocationStrayPositional(t *testing.T) {
	_, err := resolveInvocation(Options{}, []string{"bogus"}, "")
	if err == nil {
		t.Fatal("resolveInvocation: want error for stray positional, got nil")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("error = %q, want it to name the offending argument %q", err, "bogus")
	}
}

// Resolve is Run minus the serving: it must read the real process argv
// and environment, or an app using the seam resolves a different
// invocation than Run would have.
func TestResolveReadsProcessArgvAndEnv(t *testing.T) {
	orig := os.Args
	os.Args = []string{"app", "-socket", "/run/carlos/x.sock", "-db", "x.db"}
	defer func() { os.Args = orig }()
	t.Setenv("STATE_DIRECTORY", "/var/lib/app")

	opts, err := Resolve(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Socket != "/run/carlos/x.sock" {
		t.Errorf("Socket = %q", opts.Socket)
	}
	if opts.DBPath != "/var/lib/app/x.db" {
		t.Errorf("DBPath = %q, want the -db value resolved inside $STATE_DIRECTORY", opts.DBPath)
	}
}
