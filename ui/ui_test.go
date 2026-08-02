package ui

import (
	"html/template"
	"io/fs"
	"strings"
	"testing"
)

// parseAll builds the template tree exactly the way an app is documented
// to: ui.Funcs() registered, then ParseFS over Templates() with the flat
// "*.html" glob.
func parseAll(t *testing.T) *template.Template {
	t.Helper()
	tmpl, err := template.New("").Funcs(Funcs()).ParseFS(Templates(), "*.html")
	if err != nil {
		t.Fatalf("ParseFS: %v", err)
	}
	return tmpl
}

// render executes one named partial against data and returns its output.
func render(t *testing.T, name string, data any) string {
	t.Helper()
	var buf strings.Builder
	if err := parseAll(t).ExecuteTemplate(&buf, name, data); err != nil {
		t.Fatalf("ExecuteTemplate(%q): %v", name, err)
	}
	return buf.String()
}

// Templates() must hand back a filesystem rooted at the partials, not at
// the package directory — the documented ParseFS call uses "*.html".
func TestTemplatesIsRootedAtPartials(t *testing.T) {
	entries, err := fs.ReadDir(Templates(), ".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("Templates() is empty")
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".html") {
			t.Errorf("unexpected entry %q at the root of Templates()", e.Name())
		}
	}
}

func TestTokensCSSIsEmbedded(t *testing.T) {
	css := TokensCSS()
	if len(css) == 0 {
		t.Fatal("TokensCSS() is empty")
	}
	for _, want := range []string{
		"--rst-bg", "--rst-surface", "--rst-text", "--rst-accent",
		"--rst-tone-positive-fg", "--rst-sp-4",
		"prefers-color-scheme: dark", `:root[data-theme="dark"]`, `:root[data-theme="light"]`,
		".rst-status", ".rst-sr-only",
	} {
		if !strings.Contains(string(css), want) {
			t.Errorf("tokens.css is missing %q", want)
		}
	}
}

// Both themes are authored, never inverted: every themed token is
// declared three times — light, dark-by-OS, and dark-by-toggle. A
// half-authored dark theme is the classic way a token file rots.
func TestBothThemesDeclareEveryColourToken(t *testing.T) {
	css := string(TokensCSS())
	themed := []string{
		"--rst-bg", "--rst-surface", "--rst-surface-2", "--rst-line", "--rst-line-strong",
		"--rst-text", "--rst-text-muted", "--rst-text-faint",
		"--rst-accent", "--rst-accent-strong", "--rst-accent-soft", "--rst-on-accent",
		"--rst-tone-neutral-fg", "--rst-tone-neutral-bg",
		"--rst-tone-positive-fg", "--rst-tone-positive-bg",
		"--rst-tone-warning-fg", "--rst-tone-warning-bg",
		"--rst-tone-negative-fg", "--rst-tone-negative-bg",
	}
	for _, prop := range themed {
		// Declarations are "<prop>: value"; uses are "var(<prop>)", so the
		// trailing colon counts declarations only.
		if got := strings.Count(css, prop+":"); got != 3 {
			t.Errorf("%s is declared %d times, want 3 (light, prefers-color-scheme dark, [data-theme=dark])", prop, got)
		}
	}
}

func TestStatusPillRendersLabelAndTone(t *testing.T) {
	got := render(t, "status-pill", map[string]any{"Tone": "positive", "Label": "Published"})
	if !strings.Contains(got, `data-tone="positive"`) {
		t.Errorf("missing tone attribute: %s", got)
	}
	if !strings.Contains(got, "Published") {
		t.Errorf("missing visible label: %s", got)
	}
}

// State is never colour alone (spec §5, addendum §4): the label is always
// real text in the output, whatever the tone.
func TestStatusPillAlwaysCarriesTextLabel(t *testing.T) {
	for _, tone := range []string{"neutral", "positive", "warning", "negative"} {
		got := render(t, "status-pill", map[string]any{"Tone": tone, "Label": "Draft"})
		if !strings.Contains(got, ">Draft<") {
			t.Errorf("tone %q rendered no text label: %s", tone, got)
		}
	}
}

// The minimal fixture: Label only. A missing Tone falls back to neutral
// rather than rendering an empty attribute.
func TestStatusPillMinimalFixture(t *testing.T) {
	got := render(t, "status-pill", map[string]any{"Label": "Draft"})
	if !strings.Contains(got, `data-tone="neutral"`) {
		t.Errorf("missing Tone did not fall back to neutral: %s", got)
	}
	if strings.Contains(got, `data-tone=""`) {
		t.Errorf("rendered an empty tone attribute: %s", got)
	}
}
