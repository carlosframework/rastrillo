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
	want := `mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))`
	if !strings.Contains(src, want) {
		t.Errorf("main.go does not serve static/:\n%s", src)
	}
	// It has to come after the router exists, or it has nothing to attach to.
	if strings.Index(src, "gen.Router(") > strings.Index(src, want) {
		t.Error("the static handler is registered before gen.Router builds the mux")
	}
}
