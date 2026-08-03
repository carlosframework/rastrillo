package rastrillo

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// Run is the process entrypoint for a rastrillo app: it resolves the
// platform's activation argv, then serves. The platform invokes an app
// binary in two shapes (see carlosframework/platform,
// internal/activator/backend_exec.go and internal/host/units/):
//
//	<binary> [-socket p] [-addr a] [-db p]  agent exec child — hibernate
//	                                        routes; the activator spawns
//	                                        `<live> --socket <s> --db <d>`
//	<binary> serve                          carlos-app@.service unit
//	                                        tenant — no flags; the listener
//	                                        arrives via LISTEN_FDS (fd 3)
//	                                        and state lives in
//	                                        $STATE_DIRECTORY
//
// Flags override the corresponding Options fields. A relative
// Options.DBPath (or -db value) is resolved inside $STATE_DIRECTORY when
// systemd provides one — a unit tenant's cwd is not its state dir — so
// the same binary and the same Options work in a dev checkout, as an
// exec child, and as a unit tenant. Hibernation needs nothing further
// from the app: the activator owns the restore/replicate cycle, and
// Serve's SIGTERM drain (10s) fits inside the activator's 20s budget.
func Run(opts Options) error {
	opts, err := Resolve(opts)
	if err != nil {
		return err
	}
	return Serve(opts)
}

// Resolve applies the platform's activation argv and environment to
// opts — everything Run does short of serving — and returns the result.
// Most apps just call Run; Resolve is the seam for the ones that need
// the resolved invocation first. The motivating case is an app that
// opens its own database handle before building its mux (Serve's DBPath
// opens one it never hands back — see examples/blog and its friction
// log's F4): it resolves, opens opts.DBPath itself, blanks it so Serve
// won't open a second handle on the same file, and calls Serve. One
// duty transfers with the handle: Serve's own open eagerly Pings so the
// database file exists on disk from boot — a hibernate route's
// activator replicates that path from boot — so your open must touch
// the driver too (a Ping, or a migration) before Serve is called.
func Resolve(opts Options) (Options, error) {
	return resolveInvocation(opts, os.Args[1:], os.Getenv("STATE_DIRECTORY"))
}

// resolveInvocation applies one activation argv to opts. Split from Run
// so the argv/state-dir contract is testable without opening sockets.
func resolveInvocation(opts Options, args []string, stateDir string) (Options, error) {
	// The unit template execs `<binary> serve` (no flags); tolerate
	// flags after the subcommand anyway — a drop-in override may add
	// them.
	if len(args) > 0 && args[0] == "serve" {
		args = args[1:]
	}
	fs := flag.NewFlagSet("rastrillo app", flag.ContinueOnError)
	socket := fs.String("socket", "", "unix socket to listen on (platform activation contract)")
	addr := fs.String("addr", "", "TCP host:port to listen on")
	db := fs.String("db", "", "SQLite database path (activator passes the route's path here)")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if n := fs.NArg(); n > 0 {
		return opts, fmt.Errorf("unexpected argument %q — a rastrillo app accepts an optional leading \"serve\" and the -socket/-addr/-db flags", fs.Arg(0))
	}
	if *socket != "" {
		opts.Socket = *socket
	}
	if *addr != "" {
		opts.Addr = *addr
	}
	if *db != "" {
		opts.DBPath = *db
	}
	if stateDir != "" && opts.DBPath != "" && !filepath.IsAbs(opts.DBPath) {
		opts.DBPath = filepath.Join(stateDir, opts.DBPath)
	}
	return opts, nil
}
