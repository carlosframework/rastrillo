// Package generate's manifest orchestrator (this file) is the one
// entry point cmd/rastrillo calls to turn a moduleRoot's declared
// manifests into a generated tree: load, render the JSON artifact,
// run every emitter Tasks 5-9 built, run sqlc, and — in check-only
// mode — verify idempotency and route collisions without touching the
// tree at all. GenerateManifests is a complete no-op when the app
// declares no manifest resources at all (manifests are optional, per
// route — internal/manifest.Load's own doc comment): an app that
// hasn't adopted the manifest system sees no change in behavior.
package generate

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/carlosframework/rastrillo"
	"github.com/carlosframework/rastrillo/internal/manifest"
)

// GenerateManifests is the one entry cmd/rastrillo calls: load,
// artifact → gen/manifest.json, all emitters, sqlc, then the checks.
// check-only mode runs everything into a temp dir and diffs against
// gen/ (idempotency + collision without touching the tree).
func GenerateManifests(moduleRoot, genDir string, checkOnly bool) error {
	rs, err := manifest.Load(moduleRoot, filepath.Join(moduleRoot, "manifest"))
	if err != nil {
		return err
	}
	if len(rs) == 0 {
		return nil
	}

	collisions, err := routeCollisions(filepath.Join(moduleRoot, "actions"), rs)
	if err != nil {
		return err
	}
	if len(collisions) > 0 {
		return fmt.Errorf("%s%d route collision(s); build fails loudly on purpose (design doc §4)",
			FormatCollisions(collisions), len(collisions))
	}

	if !checkOnly {
		_, err := emitPipeline(moduleRoot, genDir, rs, true)
		return err
	}

	tmp, err := os.MkdirTemp("", "rastrillo-manifest-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	tmpPaths, err := emitPipeline(moduleRoot, tmp, rs, false)
	if err != nil {
		return err
	}
	return diffGenerated(tmp, genDir, tmpPaths)
}

// emitPipeline runs rs through every emitter into genDir: gen/manifest.json,
// EmitStore, (optionally) sqlc, then per-resource EmitTemplates/
// EmitActions — detecting a computed-path collision between two
// DIFFERENT resources as it goes (Name/Route uniqueness is already
// enforced by manifest.Load, but genDirFor's sanitizeIdent can still
// collapse two distinct valid routes, e.g. "/foo-bar" and "/foo_bar",
// onto the same gen/actions/ leaf directory — the one case Load's own
// uniqueness checks cannot catch) — then EmitLocales. runSqlc is false
// for check-only's temp-dir dry run: sqlc's own compiled output
// (queries.sql.go, models.go, db.go) is not part of what these
// emitters themselves produce, so diffing it would flag stale/absent
// sqlc output as a false idempotency failure, and it would force
// network/tool access on every --check. Returns every WRITTEN path
// (never a skipped one — a skip means a hand file already owns that
// path, so there is nothing of the orchestrator's own to compare
// there) for the idempotency check to diff against a second run.
func emitPipeline(moduleRoot, genDir string, rs []rastrillo.Resource, runSqlc bool) ([]string, error) {
	var written []string

	manifestPath := filepath.Join(genDir, "manifest.json")
	if err := writeFileIfChanged(manifestPath, manifest.Artifact(rs)); err != nil {
		return nil, fmt.Errorf("manifest.json: %w", err)
	}
	written = append(written, manifestPath)

	storePaths, err := EmitStore(genDir, rs)
	if err != nil {
		return nil, err
	}
	written = append(written, storePaths...)

	if runSqlc {
		if err := RunSqlc(moduleRoot); err != nil {
			return nil, err
		}
	}

	owner := map[string]string{} // computed gen path -> resource name
	claim := func(paths []string, resourceName string) error {
		for _, p := range paths {
			if prev, ok := owner[p]; ok && prev != resourceName {
				return fmt.Errorf("generated file collision: %s claimed by resources %q and %q", p, prev, resourceName)
			}
			owner[p] = resourceName
		}
		return nil
	}

	for _, r := range rs {
		tw, _, err := EmitTemplates(moduleRoot, genDir, r)
		if err != nil {
			return nil, err
		}
		if err := claim(tw, r.Name); err != nil {
			return nil, err
		}
		written = append(written, tw...)

		aw, _, err := EmitActions(moduleRoot, genDir, r)
		if err != nil {
			return nil, err
		}
		if err := claim(aw, r.Name); err != nil {
			return nil, err
		}
		written = append(written, aw...)
	}

	if err := EmitLocales(genDir, "en", rs); err != nil {
		return nil, err
	}
	written = append(written,
		filepath.Join(genDir, "locales", "en.toml"),
		filepath.Join(genDir, "locales", "locales.go"))

	return written, nil
}

// ManifestActions synthesizes the Action entries each resource's
// generated files claim: the same Route/PackageName/GenDir EmitActions
// itself computes (via the shared actionSpecs, so the two enumerations
// cannot drift apart). cmd/rastrillo/generate.go folds these into the
// same gen/router.go Router already builds for hand actions, and
// routeCollisions folds them into the same route-collision check
// Discover already runs across hand actions (design doc §4: "build
// fails loudly on any collision" — extended here to a hand action and
// a manifest resource claiming the identical route). SourcePath is a
// label, not a real actions/ file — used only to name the resource in
// a collision message or a route listing.
func ManifestActions(rs []rastrillo.Resource) ([]Action, error) {
	var out []Action
	for _, r := range rs {
		for _, s := range actionSpecs(r) {
			route, err := routeFor(s.dir, s.name, s.method)
			if err != nil {
				return nil, fmt.Errorf("resource %q: %w", r.Name, err)
			}
			base := s.name + "." + s.method + ".go"
			relSource := s.dir + "/" + base
			out = append(out, Action{
				SourcePath:  fmt.Sprintf("manifest:%s (%s)", r.Name, relSource),
				Method:      s.method,
				Route:       route,
				PackageName: packageNameFor(relSource),
				GenDir:      genDirFor(s.dir, s.name, s.method),
			})
		}
	}
	return out, nil
}

// routeCollisions merges the app's hand-written actions/ discoveries
// (if actionsDir exists at all — a manifest-only app may have none)
// with the routes each resource's own action emitter will produce, and
// reports any route claimed by more than one source. Reuses Discover's
// own Collision type and byRoute grouping, so a hand-vs-manifest
// collision prints exactly the same shape as a hand-vs-hand one.
func routeCollisions(actionsDir string, rs []rastrillo.Resource) ([]Collision, error) {
	var all []Action
	if _, err := os.Stat(actionsDir); err == nil {
		hand, _, err := Discover(actionsDir)
		if err != nil {
			return nil, err
		}
		for _, a := range hand {
			a.SourcePath = "actions/" + a.SourcePath
			all = append(all, a)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	manifestActions, err := ManifestActions(rs)
	if err != nil {
		return nil, err
	}
	all = append(all, manifestActions...)

	byRoute := map[string][]string{}
	for _, a := range all {
		byRoute[a.Route] = append(byRoute[a.Route], a.SourcePath)
	}
	var collisions []Collision
	for route, sources := range byRoute {
		if len(sources) > 1 {
			sort.Strings(sources)
			collisions = append(collisions, Collision{Route: route, Sources: sources})
		}
	}
	sort.Slice(collisions, func(i, j int) bool { return collisions[i].Route < collisions[j].Route })
	return collisions, nil
}

// FormatCollisions renders collisions the way design doc §4's "build
// fails loudly on any collision" is reported: one line naming the
// route, then every claiming source indented beneath it.
func FormatCollisions(collisions []Collision) string {
	var b strings.Builder
	b.WriteString("route collisions —\n")
	for _, c := range collisions {
		fmt.Fprintf(&b, "  %s claimed by:\n", c.Route)
		for _, s := range c.Sources {
			fmt.Fprintf(&b, "    %s\n", s)
		}
	}
	return b.String()
}

// diffGenerated byte-compares every path in tmpPaths (rooted at
// tmpDir) against its counterpart under genDir (the real, on-disk
// tree), reporting missing (expected but absent) and differing
// (present but hand-edited since generation) files. Extra-file
// detection is deliberately scoped to gen/locales/ only — the one
// subtree this orchestrator can safely walk in full: gen/store/<name>/
// also holds sqlc's own compiled output (queries.sql.go, models.go,
// db.go), and gen/actions/ is shared with hand-rewritten actions, so a
// full-tree walk of either would flag legitimate, unrelated files as
// "extra". See the task report for this scope note.
func diffGenerated(tmpDir, genDir string, tmpPaths []string) error {
	var missing, differing []string
	for _, tp := range tmpPaths {
		rel, err := filepath.Rel(tmpDir, tp)
		if err != nil {
			return err
		}
		want, err := os.ReadFile(tp)
		if err != nil {
			return err
		}
		got, err := os.ReadFile(filepath.Join(genDir, rel))
		if os.IsNotExist(err) {
			missing = append(missing, rel)
			continue
		}
		if err != nil {
			return err
		}
		if !bytes.Equal(want, got) {
			differing = append(differing, rel)
		}
	}

	extra, err := extraLocaleFiles(tmpDir, genDir)
	if err != nil {
		return err
	}

	if len(missing) == 0 && len(differing) == 0 && len(extra) == 0 {
		return nil
	}

	sort.Strings(missing)
	sort.Strings(differing)
	sort.Strings(extra)

	var b strings.Builder
	b.WriteString("generated tree is not idempotent —\n")
	if len(missing) > 0 {
		b.WriteString("  missing:\n")
		for _, f := range missing {
			fmt.Fprintf(&b, "    gen/%s\n", f)
		}
	}
	if len(extra) > 0 {
		b.WriteString("  extra:\n")
		for _, f := range extra {
			fmt.Fprintf(&b, "    gen/%s\n", f)
		}
	}
	if len(differing) > 0 {
		b.WriteString("  differing:\n")
		for _, f := range differing {
			fmt.Fprintf(&b, "    gen/%s\n", f)
		}
	}
	return errors.New(b.String())
}

// extraLocaleFiles returns the basenames present in genDir/locales but
// absent from tmpDir/locales — that directory is fully owned by
// EmitLocales (nothing else in this pipeline writes there), so a full
// listing comparison is safe.
func extraLocaleFiles(tmpDir, genDir string) ([]string, error) {
	want, err := localeFileSet(filepath.Join(tmpDir, "locales"))
	if err != nil {
		return nil, err
	}
	got, err := localeFileSet(filepath.Join(genDir, "locales"))
	if err != nil {
		return nil, err
	}
	var extra []string
	for name := range got {
		if !want[name] {
			extra = append(extra, filepath.Join("locales", name))
		}
	}
	return extra, nil
}

func localeFileSet(dir string) (map[string]bool, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			out[e.Name()] = true
		}
	}
	return out, nil
}
