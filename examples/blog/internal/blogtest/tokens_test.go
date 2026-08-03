package blogtest

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

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
