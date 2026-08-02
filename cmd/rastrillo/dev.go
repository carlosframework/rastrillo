package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/carlosframework/rastrillo/internal/devloop"
)

// watchDirs are the trees whose edits trigger the §11 loop: the design
// doc's app/, actions/, manifest/, plus cmd/ — rastrillo new scaffolds
// cmd/<name>/main.go, and a dev loop that ignores edits to it surprises
// people. gen/ is deliberately absent: it is the generator's output.
var watchDirs = []string{"actions", "app", "manifest", "cmd"}

const pollInterval = 250 * time.Millisecond

// runDev implements `rastrillo dev [dir] [-- app args...]` (design doc
// §11): watch, and on any change regenerate → rebuild → restart. It
// calls the same runGenerate that CI calls — one code path, not two.
// Everything after -- is passed to the app verbatim (e.g. -addr :9000).
func runDev(args []string) error {
	args, appArgs := splitAppArgs(args)
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}

	appPkg, appName, err := findAppPkg(dir)
	if err != nil {
		return err
	}

	binDir, err := os.MkdirTemp("", "rastrillo-dev-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(binDir)
	bin := filepath.Join(binDir, appName)

	rebuild := func() error {
		if err := runGenerate([]string{dir}); err != nil {
			return err
		}
		cmd := exec.Command("go", "build", "-o", bin, appPkg)
		cmd.Dir = dir
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		return cmd.Run()
	}

	var child *exec.Cmd
	start := func() error {
		child = exec.Command(bin, appArgs...)
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		return child.Start()
	}
	// stop SIGTERMs the child — rastrillo.Serve shuts down gracefully on
	// SIGTERM — and SIGKILLs only if it lingers past 5s.
	stop := func() {
		if child == nil || child.Process == nil {
			return
		}
		child.Process.Signal(syscall.SIGTERM)
		done := make(chan error, 1)
		go func() { done <- child.Wait() }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			child.Process.Kill()
			<-done
		}
		child = nil
	}

	// The first build must succeed — fail loudly, like generate itself.
	if err := rebuild(); err != nil {
		return err
	}
	if err := start(); err != nil {
		return err
	}
	defer stop()

	snap, err := devloop.Snapshot(dir, watchDirs)
	if err != nil {
		return err
	}

	fmt.Printf("rastrillo dev: watching %s (poll %s)\n", strings.Join(watchDirs, ", "), pollInterval)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sigCh:
			fmt.Println("rastrillo dev: shutting down")
			return nil
		case <-ticker.C:
			next, err := devloop.Snapshot(dir, watchDirs)
			if err != nil {
				fmt.Fprintf(os.Stderr, "rastrillo dev: watch: %v\n", err)
				continue
			}
			changed := devloop.Diff(snap, next)
			snap = next
			if len(changed) == 0 {
				continue
			}
			fmt.Printf("rastrillo dev: changed: %s\n", strings.Join(changed, ", "))
			if err := rebuild(); err != nil {
				// The old build keeps serving; fix and save again.
				fmt.Fprintf(os.Stderr, "rastrillo dev: %v — still serving the previous build\n", err)
				continue
			}
			stop()
			// A failed restart must not exit the loop: that would leave
			// nothing serving and require the user to manually restart
			// `rastrillo dev`, defeating §11's no-manual-intervention
			// goal. Log and keep watching; the next save retries.
			if err := start(); err != nil {
				fmt.Fprintf(os.Stderr, "rastrillo dev: start: %v — will retry on next change\n", err)
				continue
			}
		}
	}
}

// splitAppArgs separates rastrillo's own args from the app's: everything
// after the first "--" goes to the child process verbatim.
func splitAppArgs(args []string) (own, app []string) {
	for i, a := range args {
		if a == "--" {
			return args[:i], args[i+1:]
		}
	}
	return args, nil
}

// findAppPkg locates the app's main package: exactly one directory under
// cmd/, the shape `rastrillo new` scaffolds (cmd/<name>/main.go).
func findAppPkg(dir string) (pkg, name string, err error) {
	entries, err := os.ReadDir(filepath.Join(dir, "cmd"))
	if err != nil {
		return "", "", fmt.Errorf("read cmd/: %w (dev expects the rastrillo new layout: cmd/<name>/main.go)", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	if len(names) != 1 {
		return "", "", fmt.Errorf("expected exactly one directory under cmd/, found %d — dev expects the rastrillo new layout: cmd/<name>/main.go", len(names))
	}
	return "./cmd/" + names[0], names[0], nil
}
