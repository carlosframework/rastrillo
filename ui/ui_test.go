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

func TestPageHeaderMinimalFixture(t *testing.T) {
	got := render(t, "page-header", map[string]any{"Title": "Posts"})
	if !strings.Contains(got, "<h1>Posts</h1>") {
		t.Errorf("missing title: %s", got)
	}
	if strings.Contains(got, "<a ") {
		t.Errorf("no action was supplied, so no link should render: %s", got)
	}
	if strings.Contains(got, "rst-page-header__sub") {
		t.Errorf("no Sub was supplied, so no subhead should render: %s", got)
	}
}

func TestPageHeaderWithSubAndAction(t *testing.T) {
	got := render(t, "page-header", map[string]any{
		"Title":       "Posts",
		"Sub":         "Everything you have written, newest first.",
		"ActionHref":  "/posts/new",
		"ActionLabel": "Write a post",
		"ActionIcon":  "plus",
	})
	for _, want := range []string{
		"<h1>Posts</h1>",
		"Everything you have written, newest first.",
		`href="/posts/new"`,
		"Write a post",
		"<svg", // the icon resolved through Funcs()
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q: %s", want, got)
		}
	}
}

// The action is a link with visible text, so its accessible name is the
// text itself — the icon beside it must stay silent.
func TestPageHeaderActionIconIsAriaHidden(t *testing.T) {
	got := render(t, "page-header", map[string]any{
		"Title": "Posts", "ActionHref": "/posts/new",
		"ActionLabel": "Write a post", "ActionIcon": "plus",
	})
	if !strings.Contains(got, `aria-hidden="true"`) {
		t.Errorf("the action icon is not aria-hidden: %s", got)
	}
}

func TestEmptyStateMinimalFixture(t *testing.T) {
	got := render(t, "empty-state", map[string]any{
		"Body": "No posts yet. Your first one is a good place to start.",
	})
	if !strings.Contains(got, "No posts yet.") {
		t.Errorf("missing body: %s", got)
	}
	if strings.Contains(got, "<form") || strings.Contains(got, "<a ") {
		t.Errorf("no CTA was supplied, so none should render: %s", got)
	}
	if strings.Contains(got, "rst-empty__title") {
		t.Errorf("no Title was supplied, so no heading should render: %s", got)
	}
}

func TestEmptyStateLinkCTA(t *testing.T) {
	got := render(t, "empty-state", map[string]any{
		"Title":       "Nothing here yet",
		"Body":        "No posts yet. Your first one is a good place to start.",
		"ActionHref":  "/posts/new",
		"ActionLabel": "Write a post",
	})
	// A real heading, not a styled paragraph: styling a <p> to look like
	// a heading is WCAG 2.2 failure F2 (1.3.1) and heading navigation
	// skips it entirely.
	if !strings.Contains(got, `<h2 class="rst-empty__title">Nothing here yet</h2>`) {
		t.Errorf("title is not a real heading element: %s", got)
	}
	if !strings.Contains(got, `<a class="rst-btn rst-btn--primary" href="/posts/new">Write a post</a>`) {
		t.Errorf("missing link CTA: %s", got)
	}
	if strings.Contains(got, "<form") {
		t.Errorf("a link CTA must not also render a form: %s", got)
	}
}

// The POST CTA is an ordinary form: it works with JavaScript off, and
// hidden pairs (a CSRF token among them) are entirely app-supplied.
func TestEmptyStatePostCTACarriesHiddenPairs(t *testing.T) {
	got := render(t, "empty-state", map[string]any{
		"Body":        "No posts yet, and no sample data either.",
		"PostAction":  "/posts/seed",
		"ActionLabel": "Add sample posts",
		"Hidden":      [][2]string{{"csrf", "tok-123"}, {"count", "5"}},
	})
	for _, want := range []string{
		`method="post"`, `action="/posts/seed"`,
		`<input type="hidden" name="csrf" value="tok-123">`,
		`<input type="hidden" name="count" value="5">`,
		`type="submit"`, "Add sample posts",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q: %s", want, got)
		}
	}
}

// A caller that sets neither CTA, and a caller that sets a POST CTA with
// no Hidden pairs, must both render without an Execute error — ranging
// over an absent key is the classic way a partial blows up.
func TestEmptyStatePostCTAWithoutHidden(t *testing.T) {
	got := render(t, "empty-state", map[string]any{
		"Body":        "No posts yet.",
		"PostAction":  "/posts/seed",
		"ActionLabel": "Add sample posts",
	})
	if !strings.Contains(got, `action="/posts/seed"`) {
		t.Errorf("missing form action: %s", got)
	}
	if strings.Contains(got, "type=\"hidden\"") {
		t.Errorf("no Hidden pairs were supplied, so none should render: %s", got)
	}
}
