package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func scaffold(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const handleSrc = "package actions\n\nimport (\n\t\"net/http\"\n\n\t\"github.com/carlosframework/rastrillo\"\n)\n\nfunc Handle(ctx *rastrillo.Ctx, w http.ResponseWriter, r *http.Request) {}\n"

func TestGenerateWritesTheRouter(t *testing.T) {
	dir := scaffold(t, map[string]string{
		"go.mod":               "module demo\n\ngo 1.22\n",
		"actions/index.GET.go": handleSrc,
	})
	if err := runGenerate([]string{dir}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "gen", "router.go")); err != nil {
		t.Fatalf("expected gen/router.go: %v", err)
	}
}

func TestGenerateCheckWritesNothing(t *testing.T) {
	dir := scaffold(t, map[string]string{
		"go.mod":               "module demo\n\ngo 1.22\n",
		"actions/index.GET.go": handleSrc,
	})
	if err := runGenerate([]string{"--check", dir}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "gen")); !os.IsNotExist(err) {
		t.Fatal("--check must not write gen/")
	}
}

func TestGenerateWithoutCheckIgnoresAnIncompleteCatalog(t *testing.T) {
	// Plain `generate` (what `rastrillo dev` reruns on every save, and
	// what `rastrillo new` runs once) must never hard-fail on an
	// incomplete catalog — design doc §10's "silent fallback while
	// iterating" applies here; the loud failure is --check-only.
	dir := scaffold(t, map[string]string{
		"go.mod":               "module demo\n\ngo 1.22\n",
		"actions/index.GET.go": handleSrc,
		"locales/en.toml":      "a = \"A\"\nb = \"B\"\n",
		"locales/fr.toml":      "a = \"A\"\n",
	})
	if err := runGenerate([]string{dir}); err != nil {
		t.Fatalf("plain generate must not fail on an incomplete catalog: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "gen", "router.go")); err != nil {
		t.Fatalf("expected gen/router.go: %v", err)
	}
}

func TestGenerateCheckFailsOnAnIncompleteCatalog(t *testing.T) {
	dir := scaffold(t, map[string]string{
		"go.mod":               "module demo\n\ngo 1.22\n",
		"actions/index.GET.go": handleSrc,
		"locales/en.toml":      "a = \"A\"\nb = \"B\"\n",
		"locales/fr.toml":      "a = \"A\"\n",
	})
	err := runGenerate([]string{"--check", dir})
	if err == nil {
		t.Fatal("want a failure: fr.toml is missing a key en.toml has (design doc §10)")
	}
	if !strings.Contains(err.Error(), "catalog") {
		t.Errorf("error should name the check that failed: %v", err)
	}
}

func TestGenerateCheckPassesOnACompleteCatalog(t *testing.T) {
	dir := scaffold(t, map[string]string{
		"go.mod":               "module demo\n\ngo 1.22\n",
		"actions/index.GET.go": handleSrc,
		"locales/en.toml":      "a = \"A\"\n",
		"locales/fr.toml":      "a = \"A\"\n",
	})
	if err := runGenerate([]string{"--check", dir}); err != nil {
		t.Fatalf("want a pass, got %v", err)
	}
}

func TestGenerateCheckHonoursTheDefaultLocaleFlag(t *testing.T) {
	dir := scaffold(t, map[string]string{
		"go.mod":               "module demo\n\ngo 1.22\n",
		"actions/index.GET.go": handleSrc,
		"locales/fr.toml":      "a = \"A\"\nb = \"B\"\n",
		"locales/en.toml":      "a = \"A\"\n",
	})
	if err := runGenerate([]string{"--check", "--default-locale", "fr", dir}); err == nil {
		t.Fatal("with fr as the default, en.toml is the incomplete one")
	}
}

func TestGenerateWatchesLocalesAndTemplates(t *testing.T) {
	// A save to a catalog or a template must restart the dev loop, or
	// the running binary keeps serving the old embedded copy.
	for _, want := range []string{"locales", "templates"} {
		found := false
		for _, d := range watchDirs {
			if d == want {
				found = true
			}
		}
		if !found {
			t.Errorf("watchDirs is missing %q: %v", want, watchDirs)
		}
	}
}
