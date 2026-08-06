package blogtest

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	blogassets "blog"
	"github.com/carlosframework/rastrillo/ui"
)

// The app vendors the library stylesheet as static/tokens.css and
// serves it itself (rastrillo never serves CSS — F8). A vendored copy
// can silently fall behind the library — it did once, leaving the F10
// fix styled in the library and unstyled here — so pin the two
// byte-identical.
func TestVendoredTokensCSSMatchesTheLibrary(t *testing.T) {
	vendored, err := os.ReadFile(filepath.Join("..", "..", "static", "tokens.css"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(vendored, ui.TokensCSS()) {
		t.Error("static/tokens.css differs from ui.TokensCSS(); re-copy the library file")
	}
}

// The embedded static tree serves through FileServerFS exactly as the
// old http.Dir did — /static/tokens.css resolves against the embedded
// paths, which carry the static/ prefix (F8).
func TestEmbeddedStaticServesTokensCSS(t *testing.T) {
	h := http.FileServerFS(blogassets.StaticFS)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/tokens.css", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /static/tokens.css = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "--rst-bg") {
		t.Errorf("served tokens.css lacks the token block: %d bytes", rec.Body.Len())
	}
}
