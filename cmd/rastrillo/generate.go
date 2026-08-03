package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/carlosframework/rastrillo/internal/generate"
	"github.com/carlosframework/rastrillo/internal/manifest"
)

// runGenerate implements `rastrillo generate [flags] [dir]`: the
// one-shot generator rastrillo dev's watch loop and CI both call
// underneath (design doc §11) — one code path, not two.
//
// --check is the framework's half of `carlos vet` (§11): verify and
// report, write nothing. It covers route collisions (§4), the action
// build-tag convention (friction log F9), and i18n catalog completeness
// (§10) today; the rest of §11's list arrives with the subsystems it
// checks.
//
// Flags come before the directory: FlagSet.Parse stops at the first
// non-flag argument, which is what keeps the older bare `rastrillo
// generate <dir>` form working unchanged.
func runGenerate(args []string) error {
	fset := flag.NewFlagSet("generate", flag.ContinueOnError)
	check := fset.Bool("check", false, "verify without writing (route collisions, action build tags, i18n catalog completeness)")
	defaultLocale := fset.String("default-locale", "en", "locale every other catalog is checked against (design doc §10)")
	if err := fset.Parse(args); err != nil {
		return err
	}

	dir := "."
	if rest := fset.Args(); len(rest) > 0 {
		dir = rest[0]
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

	if *check {
		// Untagged action files don't break the app — plain generate
		// tolerates them, so iterating stays smooth — but they do break
		// every `go build ./...` in the tree (friction log F9), and the
		// raw go errors ("malformed import path", "Handle redeclared")
		// never mention the one-line fix. Fail here, where the message
		// can.
		untagged, err := generate.UntaggedActions(actionsDir, actions)
		if err != nil {
			return fmt.Errorf("build-tag check: %w", err)
		}
		if len(untagged) > 0 {
			fmt.Fprintln(os.Stderr, "rastrillo generate: action files missing the build constraint —")
			for _, s := range untagged {
				fmt.Fprintf(os.Stderr, "  actions/%s\n", s)
			}
			fmt.Fprintf(os.Stderr, "each needs `//go:build %s` (then a blank line) above its package clause,\n", generate.BuildTag)
			fmt.Fprintln(os.Stderr, "so `go build ./...` and friends skip generator input instead of failing on it")
			fmt.Fprintln(os.Stderr, "(a file that already carries a //go:build line needs it amended, never a second line)")
			return fmt.Errorf("%d action file(s) missing //go:build %s", len(untagged), generate.BuildTag)
		}

		missing, err := generate.MissingKeys(filepath.Join(dir, "locales"), *defaultLocale)
		if err != nil {
			return fmt.Errorf("i18n catalog check: %w", err)
		}
		if len(missing) > 0 {
			codes := make([]string, 0, len(missing))
			for code := range missing {
				codes = append(codes, code)
			}
			sort.Strings(codes)
			fmt.Fprintf(os.Stderr, "rastrillo generate: incomplete locale catalogs (default %q) —\n", *defaultLocale)
			for _, code := range codes {
				fmt.Fprintf(os.Stderr, "  locales/%s.toml is missing:\n", code)
				for _, key := range missing[code] {
					fmt.Fprintf(os.Stderr, "    %s\n", key)
				}
			}
			return fmt.Errorf("%d locale catalog(s) incomplete; silent fallback while iterating, loud failure before ship (design doc §10)", len(missing))
		}

		// Manifest resources (design doc §9): idempotency and
		// hand-vs-generated route collisions, all inside a temp dir —
		// a --check run never writes into gen/. A complete no-op when
		// the app declares no manifest resources at all.
		if err := generate.GenerateManifests(dir, genDir, true); err != nil {
			return fmt.Errorf("manifest check: %w", err)
		}

		fmt.Printf("rastrillo generate --check: %d route(s), actions tagged, locale catalogs complete\n", len(actions))
		return nil
	}

	if err := os.RemoveAll(filepath.Join(genDir, "actions")); err != nil {
		return fmt.Errorf("clear stale generated actions: %w", err)
	}
	for _, a := range actions {
		if err := generate.Rewrite(actionsDir, genDir, a); err != nil {
			return fmt.Errorf("rewrite %s: %w", a.SourcePath, err)
		}
	}

	// Manifest resources (design doc §9): gen/manifest.json, the sqlc
	// store, generated templates/actions, and the locales fragment —
	// written straight into genDir. A complete no-op when the app
	// declares no manifest resources at all (manifests are optional,
	// per route).
	if err := generate.GenerateManifests(dir, genDir, false); err != nil {
		return fmt.Errorf("generate manifests: %w", err)
	}

	// Manifest resources claim routes too (their own action files, not
	// discovered via Discover/Rewrite — see actions.go's package doc);
	// fold their Route/PackageName/GenDir into the same gen/router.go
	// Router builds for hand actions, so a resource's generated actions
	// actually get served. GenerateManifests already loaded and
	// validated the manifest set above; a manifest-less app (the
	// common case — every pre-manifest app has no manifest/ directory
	// at all) skips this second Load entirely rather than paying for
	// another goEval `go run` on every generate.
	var manifestActions []generate.Action
	if _, err := os.Stat(filepath.Join(dir, "manifest")); err == nil {
		rs, err := manifest.Load(dir, filepath.Join(dir, "manifest"))
		if err != nil {
			return err
		}
		manifestActions, err = generate.ManifestActions(actionsDir, rs)
		if err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	allActions := append(append([]generate.Action(nil), actions...), manifestActions...)

	router, err := generate.Router(module, allActions)
	if err != nil {
		return fmt.Errorf("render router.go: %w", err)
	}
	if err := os.WriteFile(filepath.Join(genDir, "router.go"), router, 0o644); err != nil {
		return err
	}

	fmt.Printf("rastrillo generate: %d route(s) wired\n", len(allActions))
	for _, a := range actions {
		fmt.Printf("  %-24s actions/%s\n", a.Route, a.SourcePath)
	}
	for _, a := range manifestActions {
		fmt.Printf("  %-24s %s\n", a.Route, a.SourcePath)
	}
	return nil
}
