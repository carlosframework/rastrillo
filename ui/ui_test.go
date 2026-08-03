package ui

import (
	"html/template"
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/carlosframework/rastrillo"
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
	// Action is set in this fixture, so it should never render empty either.
	if strings.Contains(got, ` action=""`) {
		t.Errorf("empty action attribute rendered instead of being omitted: %s", got)
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
		`<span class="rst-pagination__disabled">Previous</span>`,
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

func TestMeterClampsAndAlwaysShowsTheNumber(t *testing.T) {
	over := render(t, "meter", map[string]any{"Percent": 140, "Text": "7/5"})
	if !strings.Contains(over, "--rst-meter-fill: 100%") {
		t.Errorf("percent not clamped high: %s", over)
	}
	under := render(t, "meter", map[string]any{"Percent": -3, "Text": "0/5"})
	if !strings.Contains(under, "--rst-meter-fill: 0%") {
		t.Errorf("percent not clamped low: %s", under)
	}
	if !strings.Contains(over, `<span class="rst-meter__num">7/5</span>`) {
		t.Errorf("the fraction text is the accessible value and must render: %s", over)
	}
}

func TestCalloutTones(t *testing.T) {
	for tone, iconFrag := range map[string]string{
		"info": "M12 16v-4", "positive": "m9 12 2 2 4-4",
		"warning": "M12 9v4", "negative": "m15 9-6 6",
	} {
		got := render(t, "callout", map[string]any{"Tone": tone, "Body": "b"})
		if !strings.Contains(got, `data-tone="`+tone+`"`) || !strings.Contains(got, iconFrag) {
			t.Errorf("tone %s: wrong attribute or icon: %s", tone, got)
		}
	}
	plain := render(t, "callout", map[string]any{"Body": "b"})
	if !strings.Contains(plain, `data-tone="info"`) {
		t.Errorf("default tone is info: %s", plain)
	}
	if strings.Contains(plain, `role="alert"`) {
		t.Errorf("role=alert must be opt-in: %s", plain)
	}
	alert := render(t, "callout", map[string]any{"Body": "b", "Alert": true})
	if !strings.Contains(alert, `role="alert"`) {
		t.Errorf("Alert did not add role=alert: %s", alert)
	}
}

func TestPersonAvatarIsDecorationOnly(t *testing.T) {
	got := render(t, "person", fixtureFor(t, "person"))
	if !strings.Contains(got, `aria-hidden="true"`) {
		t.Errorf("avatar must be aria-hidden: %s", got)
	}
	empty := render(t, "person", map[string]any{"Href": "/x", "Name": "N"})
	if !strings.Contains(empty, "rst-person__av--empty") {
		t.Errorf("missing Initial renders the empty-avatar state: %s", empty)
	}
}

// allPartials is the shipped set, with a fixture exercising every
// optional field at once. Every href is deliberately relative: the
// self-containment check below bans absolute URLs outright, so anything
// absolute in the output came from a partial rather than a caller.
func allPartials() []struct {
	Name string
	Data map[string]any
} {
	return []struct {
		Name string
		Data map[string]any
	}{
		{"page-header", map[string]any{
			"Title": "Posts", "Sub": "Everything you have written, newest first.",
			"ActionHref": "/posts/new", "ActionLabel": "Write a post", "ActionIcon": "plus",
		}},
		{"list-bar", map[string]any{
			"SearchAction": "/posts", "Query": "release", "Placeholder": "Search posts",
			"Hidden": [][2]string{{"sort", "newest"}},
		}},
		{"list-bar-search", map[string]any{
			"Action": "/posts", "Query": "release", "Placeholder": "Search posts",
			"Hidden": [][2]string{{"sort", "newest"}}, "Label": "Search posts",
		}},
		{"list-search-submit", map[string]any{"Label": "Search posts"}},
		{"list-row-action", map[string]any{
			"Href": "/posts/1", "Main": "Release notes, August",
			"Sub":        "Published 2 August · 4 min read",
			"ActionHref": "/posts/1/edit", "ActionLabel": "Edit",
			"ActionAria": "Edit Release notes, August",
			"Lead":       "accent", "LeadInitial": "RN",
		}},
		{"status-pill", map[string]any{"Tone": "positive", "Label": "Published"}},
		{"empty-state", map[string]any{
			"Title": "Nothing here yet", "Body": "No posts yet. Your first one is a good place to start.",
			"PostAction": "/posts/seed", "ActionLabel": "Add sample posts",
			"Hidden": [][2]string{{"csrf", "tok-123"}},
		}},
		{"pagination", map[string]any{
			"Label": "Pagination",
			"Items": []any{
				map[string]any{"Label": "Previous", "Disabled": true},
				map[string]any{"Label": "1", "Current": true},
				map[string]any{"Label": "2", "Href": "/posts?page=2"},
				map[string]any{"Gap": true},
				map[string]any{"Label": "9", "Href": "/posts?page=9"},
			},
		}},
		{"badge", map[string]any{"Label": "Draft"}},
		{"meter", map[string]any{"Percent": 82, "Text": "412/500"}},
		{"person", map[string]any{
			"Href": "/people/1", "Name": "Grace Hopper", "Email": "grace@example.com", "Initial": "G",
		}},
		{"callout", map[string]any{
			"Tone": "warning", "Title": "Connect payments to start selling",
			"Body": "Your event is live but can't take payment yet.",
		}},
		{"detail-list", map[string]any{
			"Items": []any{
				map[string]any{"Label": "Audience", "Value": "Members"},
				map[string]any{"Label": "Main page", "Value": "No"},
			},
		}},
		{"field", map[string]any{
			"ID": "email", "Name": "email", "Label": "Email", "Type": "email",
			"Required": true, "Help": "We'll never share this.",
		}},
		{"field-select", map[string]any{
			"ID": "role", "Name": "role", "Label": "Role",
			"Options": []any{
				map[string]any{"Value": "admin", "Label": "Admin", "Selected": true},
				map[string]any{"Value": "member", "Label": "Member"},
			},
		}},
		{"field-textarea", map[string]any{
			"ID": "bio", "Name": "bio", "Label": "Bio", "Rows": "4",
			"Help": "Shown on your profile.",
		}},
		{"field-check", map[string]any{
			"Name": "notify", "Label": "Email me about replies", "Checked": true,
		}},
		{"choice-field", map[string]any{
			"Legend": "Plan", "Name": "plan",
			"Options": []any{
				map[string]any{"Value": "free", "Title": "Free", "Desc": "Good to start."},
				map[string]any{"Value": "pro", "Title": "Pro", "Desc": "For growing teams.", "Checked": true},
			},
		}},
		{"seg-tabs", map[string]any{
			"Label": "Sections",
			"Items": []any{
				map[string]any{"Label": "Basics", "Href": "?tab=basics", "Current": true},
				map[string]any{"Label": "Advanced", "Href": "?tab=advanced"},
			},
		}},
	}
}

// fixtureFor looks one fixture up by partial name. Assertions never index
// allPartials() positionally: reordering that slice would otherwise
// silently re-point a test at a different partial and still pass.
func fixtureFor(t *testing.T, name string) map[string]any {
	t.Helper()
	for _, p := range allPartials() {
		if p.Name == name {
			return p.Data
		}
	}
	t.Fatalf("no fixture defined for partial %q", name)
	return nil
}

// All partials are present and named exactly as documented.
func TestAllPartialsAreDefined(t *testing.T) {
	tmpl := parseAll(t)
	want := []string{
		"page-header", "list-bar", "list-bar-search", "list-search-submit",
		"list-row-action", "status-pill", "empty-state", "pagination",
		"badge", "meter", "person", "callout", "detail-list",
		"field", "field-select", "field-textarea", "field-check", "choice-field", "seg-tabs",
	}
	for _, name := range want {
		if tmpl.Lookup(name) == nil {
			t.Errorf("partial %q is not defined", name)
		}
	}
	if len(want) != 19 {
		t.Fatalf("the shipped set is 19 partials, this list has %d", len(want))
	}
}

// Rendered output reaches nothing outside the page: no off-origin fetch,
// no script, no remote asset. Mirrors icons_test.go's
// TestVendoredIconsAreSelfContained, adapted from one SVG string to one
// rendered partial's HTML.
func TestRenderedPartialsAreSelfContained(t *testing.T) {
	for _, p := range allPartials() {
		got := render(t, p.Name, p.Data)
		for _, bad := range []string{
			"http://", "https://", "//cdn", "<script", "<iframe", "<img",
			"<link ", "url(", "xlink:href", "@import",
		} {
			if strings.Contains(got, bad) {
				t.Errorf("partial %q reaches outside the page (%q):\n%s", p.Name, bad, got)
			}
		}
	}
}

// The one non-partial asset this package ships gets the same bar as the
// partials and the vendored icons.
func TestTokensCSSIsSelfContained(t *testing.T) {
	css := string(TokensCSS())
	for _, bad := range []string{"@import", "url(", "http://", "https://", "//fonts", "src:"} {
		if strings.Contains(css, bad) {
			t.Errorf("tokens.css reaches outside the page (%q)", bad)
		}
	}
}

// Every interactive element renders with a real accessible name
// (spec §10). Checked here rather than per-partial so a new partial
// cannot quietly opt out.
func TestEveryControlHasAnAccessibleName(t *testing.T) {
	search := render(t, "list-bar-search", fixtureFor(t, "list-bar-search"))
	if !strings.Contains(search, "aria-label=") {
		t.Errorf("the search input has no accessible name: %s", search)
	}
	if !strings.Contains(search, `type="submit">Search posts</button>`) {
		t.Errorf("the submit control has no text: %s", search)
	}
	row := render(t, "list-row-action", fixtureFor(t, "list-row-action"))
	if !strings.Contains(row, `aria-label="Edit Release notes, August"`) {
		t.Errorf("the row action pill has no disambiguating name: %s", row)
	}
	page := render(t, "pagination", fixtureFor(t, "pagination"))
	if !strings.Contains(page, `aria-label="Pagination"`) {
		t.Errorf("the pagination nav has no accessible name: %s", page)
	}
	field := render(t, "field", fixtureFor(t, "field"))
	if !strings.Contains(field, `<label class="rst-field__label" for="email">`) {
		t.Errorf("the field's input has no wired label: %s", field)
	}
	choice := render(t, "choice-field", fixtureFor(t, "choice-field"))
	if !strings.Contains(choice, "<legend>Plan</legend>") {
		t.Errorf("choice-field's legend did not render: %s", choice)
	}
	check := render(t, "field-check", fixtureFor(t, "field-check"))
	trackEnd := strings.Index(check, `</span>`)
	if trackEnd == -1 || !strings.Contains(check[trackEnd:], "Email me about replies") {
		t.Errorf("field-check's label text must render outside the aria-hidden track: %s", check)
	}
}

// The styleguide equivalent: one pass renders every partial together,
// the combined output is balanced, and each partial left its marker.
func TestRenderEverythingSmoke(t *testing.T) {
	tmpl := parseAll(t)
	var buf strings.Builder
	buf.WriteString(`<div class="rst-page">`)
	for _, p := range allPartials() {
		if err := tmpl.ExecuteTemplate(&buf, p.Name, p.Data); err != nil {
			t.Fatalf("ExecuteTemplate(%q): %v", p.Name, err)
		}
	}
	buf.WriteString(`</div>`)
	out := buf.String()

	markers := map[string]string{
		"page-header":        `<header class="rst-page-header">`,
		"list-bar":           `<div class="rst-lbar">`,
		"list-bar-search":    `<form class="rst-search"`,
		"list-search-submit": `<button class="rst-sr-only" type="submit">`,
		"list-row-action":    `<div class="rst-row">`,
		"status-pill":        `<span class="rst-status"`,
		"empty-state":        `<div class="rst-empty">`,
		"pagination":         `<nav class="rst-pagination"`,
	}
	for name, marker := range markers {
		if !strings.Contains(out, marker) {
			t.Errorf("smoke output is missing %s (%q)", name, marker)
		}
	}

	for _, tag := range []string{"div", "form", "header", "nav", "a", "p", "h1", "h2", "span", "small", "button", "svg"} {
		open, closed := countOpenTags(out, tag), strings.Count(out, "</"+tag+">")
		if open != closed {
			t.Errorf("<%s> is unbalanced: %d opened, %d closed", tag, open, closed)
		}
	}
}

// countOpenTags counts opening tags for one element name, matching both
// "<tag " and "<tag>" so <p> is never confused with <path>.
func countOpenTags(s, tag string) int {
	return strings.Count(s, "<"+tag+" ") + strings.Count(s, "<"+tag+">")
}

// F10 regression (examples/blog friction log): the class the partial
// emits for a disabled chip and the selector tokens.css styles must be
// the same string — they drifted apart once, leaving a disabled
// Previous visually identical to a live link.
func TestDisabledPaginationChipIsStyled(t *testing.T) {
	got := render(t, "pagination", fixtureFor(t, "pagination"))
	if !strings.Contains(got, `class="rst-pagination__disabled"`) {
		t.Errorf("disabled item lost its class: %s", got)
	}
	css := string(TokensCSS())
	if !strings.Contains(css, ".rst-pagination__disabled") {
		t.Errorf("tokens.css no longer styles .rst-pagination__disabled")
	}
	if strings.Contains(css, `.rst-pagination [aria-disabled=`) {
		t.Errorf("tokens.css still carries the dead aria-disabled pagination rule no partial emits")
	}
}

// Same drift check as TestDisabledPaginationChipIsStyled, extended to the
// classes the five display partials added in this batch emit: every class
// a partial can produce must have a styled selector in tokens.css, so
// nothing new ships unstyled.
func TestDisplayPartialClassesAreStyled(t *testing.T) {
	css := string(TokensCSS())
	for _, class := range []string{
		"rst-badge", "rst-badge--warning", "rst-meter", "rst-meter__bar", "rst-meter__num",
		"rst-person", "rst-person__av", "rst-callout", "rst-callout__ic", "rst-callout__body",
		"rst-detail-list",
	} {
		if !strings.Contains(css, "."+class) {
			t.Errorf("tokens.css has no selector for %q", class)
		}
	}
}

// Same drift check again, for the form family this task adds: field,
// field-select, field-textarea, field-check and choice-field between
// them can emit every one of these classes.
func TestFormPartialClassesAreStyled(t *testing.T) {
	css := string(TokensCSS())
	for _, class := range []string{
		"rst-field", "rst-field__label", "rst-field__hint", "rst-field__help", "rst-field__error",
		"rst-input", "rst-input--short",
		"rst-switch", "rst-switch__track",
		"rst-choice", "rst-choice__cards", "rst-choice__title", "rst-choice__desc",
		"rst-seg-tabs",
	} {
		if !strings.Contains(css, "."+class) {
			t.Errorf("tokens.css has no selector for %q", class)
		}
	}
}

// Help renders under the control wired via aria-describedby; Error
// replaces it (never both at once) and additionally marks the control
// aria-invalid and its own message role=alert.
func TestFieldWiresHelpAndError(t *testing.T) {
	help := render(t, "field", map[string]any{"ID": "f1", "Name": "n", "Label": "L", "Help": "h"})
	if !strings.Contains(help, `aria-describedby="f1-help"`) || !strings.Contains(help, `id="f1-help"`) {
		t.Errorf("Help not wired via aria-describedby: %s", help)
	}
	errd := render(t, "field", map[string]any{"ID": "f1", "Name": "n", "Label": "L", "Help": "h", "Error": "bad"})
	if !strings.Contains(errd, `aria-invalid="true"`) || !strings.Contains(errd, `role="alert"`) {
		t.Errorf("Error not wired: %s", errd)
	}
	if strings.Contains(errd, "f1-help") {
		t.Errorf("Error replaces Help — both rendered: %s", errd)
	}
}

// The switch is a real checkbox: keyboard and AT operate the actual
// input, and the visible track is aria-hidden decoration on top of it.
func TestFieldCheckIsARealCheckbox(t *testing.T) {
	got := render(t, "field-check", map[string]any{"Name": "on", "Label": "Enable", "Checked": true})
	for _, want := range []string{`type="checkbox"`, "checked", `aria-hidden="true"`, "rst-switch__track"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q: %s", want, got)
		}
	}
}

// Exactly one tab is aria-current at a time — the accessibility signal,
// not only the CSS that highlights it.
func TestSegTabsMarksCurrent(t *testing.T) {
	got := render(t, "seg-tabs", map[string]any{"Label": "Sections", "Items": []any{
		map[string]any{"Label": "Basics", "Href": "?tab=basics", "Current": true},
		map[string]any{"Label": "Advanced", "Href": "?tab=advanced"},
	}})
	if !strings.Contains(got, `aria-current="page">Basics`) {
		t.Errorf("current tab unmarked: %s", got)
	}
	if strings.Count(got, "aria-current") != 1 {
		t.Errorf("exactly one current tab: %s", got)
	}
}

// iconSVG returns one vendored icon's markup as a plain string for
// building styleguide samples inline. rastrillo.Icon returns
// template.HTML; string() is safe here because every argument this
// package's tests pass is a compile-time constant slug, never
// request-derived text.
func iconSVG(slug string) string { return string(rastrillo.Icon(slug)) }

// styleguideSamples are the canonical markup samples for the class
// idioms — structural components with arbitrary bodies that a Go
// template partial cannot wrap. The smoke test renders them so every
// documented class is exercised, and the class↔css test keeps them
// honest against tokens.css (the F10 lesson, generalized). ui.go's
// package doc references this map by name rather than duplicating the
// markup, so the two cannot drift.
var styleguideSamples = map[string]string{
	"box": `<div class="rst-box-head"><h2>Payout</h2><a class="rst-btn" href="/payout/edit">Edit</a></div>
<section class="rst-box"><p>Everything on a screen sits inside boxes.</p><div class="rst-box-foot">Last updated 2 hours ago</div></section>`,
	"list-grid": `<div class="rst-card" style="--rst-cols: 2fr 110px 32px">
  <div class="rst-lrow rst-lrow--head"><span>Order</span><span class="rst-m-hide">Status</span><span></span></div>
  <div class="rst-lrow">
    <a class="rst-nm" href="/orders/AB3PX">Grace Hopper<small>AB3PX · grace@example.com</small></a>
    <span class="rst-m-hide rst-cell-mut">Paid</span>
    <details class="rst-row-menu"><summary aria-label="Actions for order AB3PX">` + iconSVG("kebab") + `</summary>
      <div class="rst-row-menu__panel"><a href="/orders/AB3PX">View</a><hr><button type="submit" class="rst-danger">Refund order…</button></div>
    </details>
  </div>
  <p class="rst-no-match">No orders match. <a href="/orders">Clear filters</a></p>
</div>
<p class="rst-count-line">Displaying <strong>1–20</strong> of <strong>412</strong></p>`,
	"dropdown": `<details class="rst-dropdown" name="list-controls">
  <summary>Filter<span class="rst-caret" aria-hidden="true">` + iconSVG("chevron-down") + `</span><span class="rst-sr-only">Filter orders: Paid</span></summary>
  <div class="rst-dropdown__menu">
    <a aria-current="true" href="/orders?status=paid">Paid</a>
    <details class="rst-menu-group" open><summary>Price</summary><div><a href="/orders?price=free">Free</a></div></details>
  </div>
</details>
<span class="rst-ftok"><span class="rst-ftok__k">Paid</span><a href="/orders" aria-label="Remove filter Paid">✕</a></span>`,
	// form-layout demonstrates the classes tokens.css ships for form
	// rhythm and the save bar (rst-form-flow, rst-field-row, rst-grow,
	// rst-form-foot, rst-form-actions) — no partial emits these, since
	// they wrap a caller-composed run of "field" partials rather than a
	// single data shape. Two adjacent .rst-field divs exercise the
	// rst-form-flow spacing rule; the row's grown field exercises
	// rst-grow. The cancel/save pair reuses the existing button classes
	// (Task 3's ambiguity resolution: no new rst-btn variant needed).
	"form-layout": `<form class="rst-form-flow" method="post" action="/settings">
  <div class="rst-field">
    <label class="rst-field__label" for="name">Name</label>
    <input class="rst-input" type="text" id="name" name="name">
  </div>
  <div class="rst-field">
    <label class="rst-field__label" for="email">Email</label>
    <input class="rst-input" type="email" id="email" name="email">
  </div>
  <div class="rst-field-row">
    <div class="rst-field rst-grow">
      <label class="rst-field__label" for="city">City</label>
      <input class="rst-input" type="text" id="city" name="city">
    </div>
    <div class="rst-field">
      <label class="rst-field__label" for="zip">ZIP</label>
      <input class="rst-input rst-input--short" type="text" id="zip" name="zip">
    </div>
  </div>
  <div class="rst-form-foot">
    <span class="rst-form-foot__note">Changes save immediately.</span>
    <div class="rst-form-actions">
      <a class="rst-btn" href="/settings">Cancel</a>
      <button class="rst-btn rst-btn--primary" type="submit">Save</button>
    </div>
  </div>
</form>`,
}

// The samples are static HTML with no template actions, so parsing them
// through the ui funcs is enough to prove they are well-formed
// standalone markup a styleguide page can Execute verbatim.
func TestStyleguideSamplesRender(t *testing.T) {
	for name, sample := range styleguideSamples {
		tmpl, err := template.New(name).Funcs(Funcs()).Parse(sample)
		if err != nil {
			t.Fatalf("%s: Parse: %v", name, err)
		}
		var buf strings.Builder
		if err := tmpl.Execute(&buf, nil); err != nil {
			t.Fatalf("%s: Execute: %v", name, err)
		}
		out := buf.String()
		if out == "" {
			t.Errorf("%s: rendered empty", name)
		}
		for _, tag := range []string{"div", "details", "section", "a", "span"} {
			open, closed := countOpenTags(out, tag), strings.Count(out, "</"+tag+">")
			if open != closed {
				t.Errorf("%s: <%s> is unbalanced: %d opened, %d closed", name, tag, open, closed)
			}
		}
	}
}

// rstClassPattern extracts one rst- class token, including its optional
// BEM __element and --modifier suffixes.
var rstClassPattern = regexp.MustCompile(`rst-[a-z-]+(?:__[a-z-]+)?(?:--[a-z-]+)?`)

// classAttrPattern isolates class="..." attribute values, so extraction
// runs over actual class tokens rather than the whole sample string —
// the list-grid sample's inline `style="--rst-cols: …"` also matches
// rstClassPattern (as "rst-cols"), but --rst-cols is a custom property
// read with var(), never a class selector, and checking it against
// tokens.css with a leading "." would be a false positive.
var classAttrPattern = regexp.MustCompile(`class="([^"]*)"`)

// TestIdiomClassesAreStyled is the F10 lesson in both directions: every
// class a sample emits must have a selector in tokens.css (a sample
// cannot reference a class that does not exist), and every selector this
// task added must be exercised by some sample (an idiom cannot ship
// undemonstrated).
func TestIdiomClassesAreStyled(t *testing.T) {
	css := string(TokensCSS())
	seen := map[string]bool{}
	for _, sample := range styleguideSamples {
		for _, attr := range classAttrPattern.FindAllStringSubmatch(sample, -1) {
			for _, class := range rstClassPattern.FindAllString(attr[1], -1) {
				seen[class] = true
			}
		}
	}
	for class := range seen {
		if !strings.Contains(css, "."+class) {
			t.Errorf("tokens.css has no selector for %q (used in a styleguide sample)", class)
		}
	}

	// The selectors this task's Step 1 added to tokens.css, listed
	// literally: each one must appear in at least one sample above.
	for _, class := range []string{
		"rst-box", "rst-box-head", "rst-box-foot",
		"rst-card", "rst-lrow", "rst-lrow--head", "rst-m-hide", "rst-nm", "rst-cell-mut",
		"rst-no-match", "rst-count-line",
		"rst-row-menu", "rst-row-menu__panel", "rst-danger",
		"rst-dropdown", "rst-dropdown__menu", "rst-menu-group", "rst-caret",
		"rst-ftok",
	} {
		if !seen[class] {
			t.Errorf("selector %q was added to tokens.css this task but no styleguide sample uses it", class)
		}
	}

	// Task 3's form-layout selectors: no partial emits these (they wrap a
	// caller-composed run of fields, not a single data shape), so the
	// "form-layout" sample above is their only exercise.
	for _, class := range []string{
		"rst-form-flow", "rst-field-row", "rst-grow", "rst-form-foot", "rst-form-foot__note", "rst-form-actions",
	} {
		if !seen[class] {
			t.Errorf("selector %q was added to tokens.css in the form-layout task but no styleguide sample uses it", class)
		}
	}
}

// The dropdown's exclusivity between siblings (only one open at a time)
// is the native <details name> attribute, not JavaScript — this pins
// both halves of that promise.
func TestDropdownExclusivityIsNative(t *testing.T) {
	sample := styleguideSamples["dropdown"]
	if !strings.Contains(sample, `<details class="rst-dropdown" name=`) {
		t.Errorf("dropdown sample's outer <details> carries no name attribute: %s", sample)
	}
	if strings.Contains(sample, "<script") {
		t.Errorf("dropdown sample reaches for <script>; exclusivity must stay native: %s", sample)
	}
}
