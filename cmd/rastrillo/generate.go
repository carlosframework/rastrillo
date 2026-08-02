package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/carlosframework/rastrillo/internal/generate"
)

// runGenerate implements `rastrillo generate [dir]`: the one-shot
// generator rastrillo dev's watch loop and CI both call underneath
// (design doc §11) — not yet wired to a watcher tonight, but this is
// the single code path either would drive.
func runGenerate(args []string) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}

	module, err := modulePath(dir)
	if err != nil {
		return err
	}

	actionsDir := filepath.Join(dir, "actions")
	if _, err := os.Stat(actionsDir); os.IsNotExist(err) {
		return fmt.Errorf("no actions/ directory in %s", dir)
	}

	actions, collisions, err := generate.Discover(actionsDir)
	if err != nil {
		return fmt.Errorf("discover actions: %w", err)
	}
	if len(collisions) > 0 {
		fmt.Fprintln(os.Stderr, "rastrillo generate: route collisions —")
		for _, c := range collisions {
			fmt.Fprintf(os.Stderr, "  %s claimed by:\n", c.Route)
			for _, s := range c.Sources {
				fmt.Fprintf(os.Stderr, "    actions/%s\n", s)
			}
		}
		return fmt.Errorf("%d route collision(s); build fails loudly on purpose (design doc §4)", len(collisions))
	}

	genDir := filepath.Join(dir, "gen")
	if err := os.RemoveAll(filepath.Join(genDir, "actions")); err != nil {
		return fmt.Errorf("clear stale generated actions: %w", err)
	}
	for _, a := range actions {
		if err := generate.Rewrite(actionsDir, genDir, a); err != nil {
			return fmt.Errorf("rewrite %s: %w", a.SourcePath, err)
		}
	}

	router, err := generate.Router(module, actions)
	if err != nil {
		return fmt.Errorf("render router.go: %w", err)
	}
	if err := os.WriteFile(filepath.Join(genDir, "router.go"), router, 0o644); err != nil {
		return err
	}

	fmt.Printf("rastrillo generate: %d route(s) wired\n", len(actions))
	for _, a := range actions {
		fmt.Printf("  %-24s actions/%s\n", a.Route, a.SourcePath)
	}
	return nil
}
