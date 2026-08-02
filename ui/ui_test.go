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

func TestListBarSearchMinimalFixture(t *testing.T) {
	got := render(t, "list-bar-search", map[string]any{"Action": "/posts"})
	for _, want := range []string{
		`<form class="rst-search"`, `role="search"`, `method="get"`, `action="/posts"`,
		`type="search"`, `name="q"`, `<svg`, `type="submit"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q: %s", want, got)
		}
	}
	// No Query, no Placeholder: neither attribute should appear empty.
	if strings.Contains(got, `value=""`) || strings.Contains(got, `placeholder=""`) {
		t.Errorf("empty attributes rendered instead of being omitted: %s", got)
	}
}

// The input always has a real accessible name, whether or not the caller
// supplied a placeholder.
func TestListBarSearchInputAlwaysHasAnAccessibleName(t *testing.T) {
	bare := render(t, "list-bar-search", map[string]any{"Action": "/posts"})
	if !strings.Contains(bare, `aria-label="Search"`) {
		t.Errorf("no default accessible name on the input: %s", bare)
	}
	named := render(t, "list-bar-search", map[string]any{
		"Action": "/posts", "Placeholder": "Search posts",
	})
	if !strings.Contains(named, `aria-label="Search posts"`) {
		t.Errorf("Placeholder did not become the accessible name: %s", named)
	}
	if !strings.Contains(named, `placeholder="Search posts"`) {
		t.Errorf("missing placeholder attribute: %s", named)
	}
}

func TestListBarSearchCarriesQueryAndHidden(t *testing.T) {
	got := render(t, "list-bar-search", map[string]any{
		"Action": "/posts",
		"Query":  "release notes",
		"Hidden": [][2]string{{"sort", "newest"}},
	})
	if !strings.Contains(got, `value="release notes"`) {
		t.Errorf("query not preserved: %s", got)
	}
	if !strings.Contains(got, `<input type="hidden" name="sort" value="newest">`) {
		t.Errorf("hidden pair not preserved across the search GET: %s", got)
	}
}

// The submit control is present, is a real button, and defaults to
// "Search" — a keyboard user with JS off has to be able to submit.
func TestListSearchSubmitDefaultsAndOverrides(t *testing.T) {
	def := render(t, "list-search-submit", map[string]any{})
	if !strings.Contains(def, `<button class="rst-sr-only" type="submit">Search</button>`) {
		t.Errorf("default submit control is wrong: %s", def)
	}
	over := render(t, "list-search-submit", map[string]any{"Label": "Buscar"})
	if !strings.Contains(over, ">Buscar<") {
		t.Errorf("Label override ignored: %s", over)
	}
}

func TestListBarSearchPassesLabelThroughToSubmit(t *testing.T) {
	got := render(t, "list-bar-search", map[string]any{"Action": "/posts", "Label": "Buscar"})
	if !strings.Contains(got, ">Buscar<") {
		t.Errorf("Label did not reach list-search-submit: %s", got)
	}
}

func TestListBarWrapsTheSearchFormInAToolbarStrip(t *testing.T) {
	got := render(t, "list-bar", map[string]any{
		"SearchAction": "/posts",
		"Query":        "notes",
		"Placeholder":  "Search posts",
		"Hidden":       [][2]string{{"sort", "newest"}},
	})
	for _, want := range []string{
		`<div class="rst-lbar">`, `<form class="rst-search"`,
		`action="/posts"`, `value="notes"`, `placeholder="Search posts"`,
		`<input type="hidden" name="sort" value="newest">`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q: %s", want, got)
		}
	}
	// This slice renders no filter or sort control (spec §3).
	if strings.Contains(got, "<details") {
		t.Errorf("list-bar rendered a dropdown, which is a later slice: %s", got)
	}
}

// A list-bar built with only its action — the minimal fixture. Every
// other key is absent, so this is where a stray empty attribute or a
// lost default would show up.
func TestListBarMinimalFixture(t *testing.T) {
	got := render(t, "list-bar", map[string]any{"SearchAction": "/posts"})
	if !strings.Contains(got, `<div class="rst-lbar">`) {
		t.Errorf("missing toolbar strip: %s", got)
	}
	if !strings.Contains(got, `action="/posts"`) {
		t.Errorf("missing form action: %s", got)
	}
	// The default accessible name survives the trip through list-bar's dict.
	if !strings.Contains(got, `aria-label="Search"`) {
		t.Errorf("the search input lost its default accessible name: %s", got)
	}
	if strings.Contains(got, `value=""`) || strings.Contains(got, `placeholder=""`) {
		t.Errorf("empty attributes rendered instead of being omitted: %s", got)
	}
}

func TestListRowActionMinimalFixture(t *testing.T) {
	got := render(t, "list-row-action", map[string]any{
		"Href": "/posts/1", "Main": "Release notes, August",
	})
	if !strings.Contains(got, `<a href="/posts/1">Release notes, August</a>`) {
		t.Errorf("missing primary link: %s", got)
	}
	if strings.Contains(got, "rst-row__lead") || strings.Contains(got, "rst-row__action") {
		t.Errorf("optional parts rendered without data: %s", got)
	}
	if strings.Contains(got, "rst-row__sub") {
		t.Errorf("no Sub was supplied, so no meta line should render: %s", got)
	}
}

func TestListRowActionFullFixture(t *testing.T) {
	got := render(t, "list-row-action", map[string]any{
		"Href": "/posts/1", "Main": "Release notes, August",
		"Sub":        "Published 2 August · 4 min read",
		"ActionHref": "/posts/1/edit", "ActionLabel": "Edit",
		"ActionAria": "Edit Release notes, August",
		"Lead":       "positive", "LeadInitial": "RN",
	})
	for _, want := range []string{
		`data-lead="positive"`, `aria-hidden="true"`, ">RN<",
		"Published 2 August · 4 min read",
		`href="/posts/1/edit"`, `aria-label="Edit Release notes, August"`, ">Edit<",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q: %s", want, got)
		}
	}
}

// Two separate anchors, never one nested inside the other: nested
// anchors are invalid HTML and the inner one is unreachable by keyboard.
func TestListRowActionNeverNestsAnchors(t *testing.T) {
	got := render(t, "list-row-action", map[string]any{
		"Href": "/posts/1", "Main": "Release notes",
		"ActionHref": "/posts/1/edit", "ActionLabel": "Edit",
	})
	first := strings.Index(got, "<a ")
	firstClose := strings.Index(got, "</a>")
	second := strings.LastIndex(got, "<a ")
	if first == -1 || firstClose == -1 || second == first {
		t.Fatalf("expected two anchors: %s", got)
	}
	if second < firstClose {
		t.Errorf("the action anchor opens before the name anchor closes: %s", got)
	}
}

func TestPaginationRendersEveryItemKind(t *testing.T) {
	got := render(t, "pagination", map[string]any{
		"Items": []any{
			map[string]any{"Label": "Previous", "Disabled": true},
			map[string]any{"Label": "1", "Current": true},
			map[string]any{"Label": "2", "Href": "/posts?page=2"},
			map[string]any{"Gap": true},
			map[string]any{"Label": "9", "Href": "/posts?page=9"},
			map[string]any{"Label": "Next", "Href": "/posts?page=2"},
		},
	})
	for _, want := range []string{
		`aria-label="Pagination"`,
		`<span aria-disabled="true">Previous</span>`,
		`<span aria-current="page">1</span>`,
		`<a href="/posts?page=2">2</a>`,
		`aria-hidden="true"`,
		`<a href="/posts?page=9">9</a>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q: %s", want, got)
		}
	}
	// The gap item carries no Label, and a gap is never a link.
	if strings.Contains(got, `<a href="">`) {
		t.Errorf("the gap item rendered as an empty link: %s", got)
	}
}

// The current page is marked in the accessibility tree, not only in the
// stylesheet.
func TestPaginationCurrentPageIsNotColourAlone(t *testing.T) {
	got := render(t, "pagination", map[string]any{
		"Items": []any{map[string]any{"Label": "3", "Current": true}},
	})
	if !strings.Contains(got, `aria-current="page"`) {
		t.Errorf("current page carries no aria-current: %s", got)
	}
}

func TestPaginationLabelOverride(t *testing.T) {
	got := render(t, "pagination", map[string]any{
		"Label": "Paginación",
		"Items": []any{map[string]any{"Label": "1", "Current": true}},
	})
	if !strings.Contains(got, `aria-label="Paginación"`) {
		t.Errorf("Label override ignored: %s", got)
	}
}

// An empty page strip must render an empty nav, not an Execute error.
func TestPaginationWithNoItems(t *testing.T) {
	got := render(t, "pagination", map[string]any{})
	if !strings.Contains(got, `<nav class="rst-pagination"`) {
		t.Errorf("missing nav: %s", got)
	}
	if strings.Contains(got, "<a ") {
		t.Errorf("no items were supplied, so no links should render: %s", got)
	}
}
