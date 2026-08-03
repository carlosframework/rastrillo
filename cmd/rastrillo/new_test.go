package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carlosframework/rastrillo/ui"
)

// rastrillo new writes the design-token stylesheet into the new app's
// static tree, once. From then on it is an ordinary app-owned file.
func TestNewScaffoldsTokensCSS(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := runNew([]string{"blogapp"}); err != nil {
		t.Fatalf("runNew: %v", err)
	}
	got, err := os.ReadFile(filepath.Join("blogapp", "static", "tokens.css"))
	if err != nil {
		t.Fatalf("expected a scaffolded stylesheet: %v", err)
	}
	if !bytes.Equal(got, ui.TokensCSS()) {
		t.Errorf("scaffolded tokens.css is not ui.TokensCSS() verbatim (%d bytes vs %d)", len(got), len(ui.TokensCSS()))
	}
}

// The scaffolded app still builds the same way it did before: go.mod,
// the starter action, main.go, and a generated router.
func TestNewStillScaffoldsTheRestOfTheApp(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := runNew([]string{"blogapp"}); err != nil {
		t.Fatalf("runNew: %v", err)
	}
	for _, rel := range []string{
		"go.mod",
		filepath.Join("actions", "index.GET.go"),
		filepath.Join("cmd", "blogapp", "main.go"),
		filepath.Join("gen", "router.go"),
	} {
		if _, err := os.Stat(filepath.Join("blogapp", rel)); err != nil {
			t.Errorf("missing scaffolded file %s: %v", rel, err)
		}
	}
}

// The generated app serves its own static directory. rastrillo.Serve
// never serves CSS — that is the app's job, in the app's own code.
func TestMainTemplateServesTheStaticDir(t *testing.T) {
	src := fmt.Sprintf(mainTemplate, "blogapp")
	want := `mux.Handle("GET /static/", http.FileServerFS(app.StaticFS))`
	if !strings.Contains(src, want) {
		t.Errorf("main.go does not serve static/:\n%s", src)
	}
	// It has to come after the router exists, or it has nothing to attach to.
	if strings.Index(src, "gen.Router(") > strings.Index(src, want) {
		t.Error("the static handler is registered before gen.Router builds the mux")
	}
}

// The scaffolded app embeds static/ — the platform deploys the binary
// alone, so a loose static directory would not travel with it (F8).
func TestNewScaffoldsEmbeddedStatic(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := runNew([]string{"blogapp"}); err != nil {
		t.Fatalf("runNew: %v", err)
	}
	src, err := os.ReadFile(filepath.Join("blogapp", "assets.go"))
	if err != nil {
		t.Fatalf("expected a scaffolded assets.go: %v", err)
	}
	for _, want := range []string{"//go:embed static", "var StaticFS embed.FS", "package blogapp"} {
		if !strings.Contains(string(src), want) {
			t.Errorf("assets.go missing %q:\n%s", want, src)
		}
	}
}

// packageName derives a Go identifier from the app name for the
// scaffolded root package, since the name is also the module path
// where hyphens (and other non-identifier characters) are legal.
func TestPackageName(t *testing.T) {
	cases := []struct{ name, want string }{
		{"blogapp", "blogapp"},
		{"my-blog", "myblog"},
		{"9lives", "app9lives"},
		{"--", "app"},
	}
	for _, c := range cases {
		if got := packageName(c.name); got != c.want {
			t.Errorf("packageName(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

// F9 end to end: a fresh scaffold's action file carries the build
// constraint, the gen copy doesn't, and --check passes as scaffolded.
func TestNewScaffoldsTaggedActions(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := runNew([]string{"tagapp"}); err != nil {
		t.Fatalf("runNew: %v", err)
	}
	src, err := os.ReadFile(filepath.Join("tagapp", "actions", "index.GET.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "//go:build rastrillo_actions") {
		t.Errorf("scaffolded action lacks the build constraint:\n%s", src)
	}
	gen, err := os.ReadFile(filepath.Join("tagapp", "gen", "actions", "index_get", "index.GET.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(gen), "//go:build") {
		t.Errorf("gen copy kept the build constraint:\n%s", gen)
	}
	if err := runGenerate([]string{"--check", "tagapp"}); err != nil {
		t.Errorf("--check must pass on a fresh scaffold: %v", err)
	}
}
