package blog

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/carlosframework/rastrillo"
	"github.com/carlosframework/rastrillo/ui"
)

//go:embed templates/layout.html templates/pages/*.html
var templateFS embed.FS

// PageSize is how many posts a list screen shows. Ten is small enough
// that eleven posts produce a real page strip in a test.
const PageSize = 10

// pages is one template tree per screen, built once at process start.
var pages = buildPages()

// buildPages parses ui's partials and the layout into a base tree, then
// clones that base once per page file.
//
// The clone is what makes the layout work: every page file defines
// "content", the same name in every file. Parsed into one shared tree
// they would overwrite each other silently — last file wins, wrong screen
// rendered, no error anywhere. A clone per page gives each screen its own
// "content", so layout.html can call {{template "content" .}} with the
// constant name html/template requires.
func buildPages() map[string]*template.Template {
	base := template.Must(template.New("").Funcs(ui.Funcs()).ParseFS(ui.Templates(), "*.html"))
	base = template.Must(base.ParseFS(templateFS, "templates/layout.html"))

	names, err := fs.Glob(templateFS, "templates/pages/*.html")
	if err != nil {
		panic(err) // the pattern is a constant and the FS is embedded
	}
	if len(names) == 0 {
		panic("blog: no page templates embedded")
	}
	out := make(map[string]*template.Template, len(names))
	for _, n := range names {
		t := template.Must(base.Clone())
		t = template.Must(t.ParseFS(templateFS, n))
		out[strings.TrimSuffix(filepath.Base(n), ".html")] = t
	}
	return out
}

// Render executes one page's "layout" into a buffer and only then writes
// the status and the bytes, so a template error is a clean 500 rather
// than half a page followed by a stack trace.
func Render(ctx *rastrillo.Ctx, w http.ResponseWriter, page string, status int, data any) {
	t, ok := pages[page]
	if !ok {
		Fail(ctx, w, "rendering "+page, fmt.Errorf("no such page template"))
		return
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "layout", data); err != nil {
		Fail(ctx, w, "rendering "+page, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	w.Write(buf.Bytes())
}

// Fail logs through Ctx.Logger and answers a plain 500. Every store error
// that is not sql.ErrNoRows goes through it in one line.
func Fail(ctx *rastrillo.Ctx, w http.ResponseWriter, what string, err error) {
	if ctx != nil && ctx.Logger != nil {
		ctx.Logger.Error("blog: "+what, "err", err)
	}
	http.Error(w, "Something went wrong.", http.StatusInternalServerError)
}

// ParseID reads the {id} path value. A non-numeric id is a URL that was
// never ours, so the caller answers 404 rather than 400.
func ParseID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, false
	}
	return id, true
}

// PageParam reads ?page=, defaulting to 1 for anything missing or silly.
func PageParam(r *http.Request) int {
	n, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// Offset is the LIMIT/OFFSET offset for a 1-based page number.
func Offset(page int) int { return (page - 1) * PageSize }

// ── View models ────────────────────────────────────────────────────────
//
// Every screen's data is a Go struct, never a dict-built map: a mistyped
// field on a struct fails loudly at Execute, while a missing map key
// renders empty and returns no error. dict stays in the templates, where
// it belongs — for the literal argument lists that call a partial.

// Head carries what the layout's <head> needs.
type Head struct{ Title string }

// Row is one list-row-action's data, formatted in Go.
type Row struct {
	Href        string
	Main        string
	Sub         string
	ActionHref  string
	ActionLabel string
	ActionAria  string
}

// PageItem is one control in the pagination strip. Exactly one of Href,
// Current, Disabled and Gap is meaningful per item.
type PageItem struct {
	Label    string
	Href     string
	Current  bool
	Disabled bool
	Gap      bool
}

// Pagination is the strip plus the app's own decision to show it at all:
// the partial happily renders an empty <nav>, which would leave a
// landmark on every single-page list.
type Pagination struct {
	Show  bool
	Items []PageItem
}

// HomeView is GET /.
type HomeView struct {
	Head       Head
	Rows       []Row
	Pagination Pagination
}

// PostView is GET /posts/{id}.
type PostView struct {
	Head       Head
	Title      string
	Date       string
	Paragraphs []string
}

// AdminListView is GET /admin/posts.
type AdminListView struct {
	Head  Head
	Query string
	// Carry is list-bar's Hidden: name/value pairs a search must
	// preserve. The handler sets it to [][2]string{{"status", status}}
	// when a filter is applied, so a search from a filtered list keeps
	// it — a new search still deliberately returns to page 1.
	Carry       [][2]string
	Filter      Filter
	NoMatchNote string
	Rows        []Row
	Pagination  Pagination
	Empty       bool // no posts at all: the real blank state
	NoMatch     bool // a search or filter that matched nothing: a plain note, not a card
}

// FilterItem is one dropdown choice; Filter is list-bar's Filter value.
// The field names match the dropdown partial's key contract.
type FilterItem struct {
	Href    string
	Label   string
	Current bool
}

type Filter struct {
	Label string
	Aria  string
	Items []FilterItem
}

// statusLabels resolves a normalized status to its visible label.
var statusLabels = map[string]string{"": "All", "draft": "Drafts", "published": "Published"}

// NormalizeStatus maps a raw query value onto the three states the
// screen has. Anything unrecognized is "all", not an error: a stale
// bookmark should show posts, not a 400.
func NormalizeStatus(raw string) string {
	if raw == "draft" || raw == "published" {
		return raw
	}
	return ""
}

// BuildStatusFilter builds the admin list's status dropdown. Hrefs
// carry the current search and reset paging — changing a filter starts
// at page 1 by construction.
func BuildStatusFilter(q, status string) Filter {
	href := func(s string) string {
		var params []string
		if q != "" {
			params = append(params, "q="+url.QueryEscape(q))
		}
		if s != "" {
			params = append(params, "status="+s)
		}
		if len(params) == 0 {
			return "/admin/posts"
		}
		return "/admin/posts?" + strings.Join(params, "&")
	}
	f := Filter{
		Label: statusLabels[status],
		Aria:  "Filter by status: " + statusLabels[status],
	}
	for _, s := range []string{"", "draft", "published"} {
		f.Items = append(f.Items, FilterItem{Href: href(s), Label: statusLabels[s], Current: s == status})
	}
	return f
}

// NoMatchNote words the "nothing matched" note for the applied search
// and filter. Formatting stays in Go, where a test reaches it.
func NoMatchNote(q, status string) string {
	subject := map[string]string{"": "posts", "draft": "drafts", "published": "published posts"}[status]
	if q != "" {
		return fmt.Sprintf("No %s match “%s”.", subject, q)
	}
	return fmt.Sprintf("No %s yet.", subject)
}

// AdminFormView is GET /admin/posts/new and GET /admin/posts/{id}/edit,
// and the 400 re-render of either POST.
type AdminFormView struct {
	Head        Head
	Title       string // page-header title (the post's own, when editing)
	Sub         string // page-header subhead
	Action      string // the form's POST target
	FormTitle   string // the title field's current value
	Body        string // the textarea's current value
	Error       string // a validation message, rendered role="alert"
	ID          int64
	Published   bool
	StatusTone  string // status-pill Tone: neutral or positive
	StatusLabel string // status-pill Label: Draft or Published
}

// EditForm builds the edit screen's view model from a post. Resolving a
// raw published flag to a Tone/Label pair is product logic and stays
// here, in the app: status-pill takes the resolved pair by design.
func EditForm(p Post) AdminFormView {
	tone, label := "neutral", "Draft"
	if p.Published {
		tone, label = "positive", "Published"
	}
	return AdminFormView{
		Head:        Head{Title: p.Title},
		Title:       p.Title,
		Sub:         "Editing",
		Action:      fmt.Sprintf("/admin/posts/%d", p.ID),
		FormTitle:   p.Title,
		Body:        p.Body,
		ID:          p.ID,
		Published:   p.Published,
		StatusTone:  tone,
		StatusLabel: label,
	}
}

// ── Formatting, all of it in Go ────────────────────────────────────────
//
// The app registers no template funcs of its own: ui.Funcs() is the whole
// FuncMap. Every date, every meta line, every page item is computed here,
// where a test reaches it directly.

// FormatDate renders a timestamp the way the screens read it.
func FormatDate(t time.Time) string { return t.Format("2 January 2006") }

// PublishedLine is a published post's meta line.
func PublishedLine(t time.Time) string { return "Published " + FormatDate(t) }

// DraftLine is a draft's meta line. Status rides in the row's Sub as
// prose because the stock row has no status slot — friction finding F1.
func DraftLine(t time.Time) string { return "Draft · edited " + FormatDate(t) }

// PublicRows builds the public index's rows. No action pill: the row
// already goes to the only place it could go, and a second link to the
// same URL is noise with a tab stop.
func PublicRows(posts []Post) []Row {
	rows := make([]Row, 0, len(posts))
	for _, p := range posts {
		rows = append(rows, Row{
			Href: fmt.Sprintf("/posts/%d", p.ID),
			Main: p.Title,
			Sub:  FormatDate(p.CreatedAt),
		})
	}
	return rows
}

// AdminRows builds the admin list's rows. A draft gets no action pill:
// it has no public page to send anyone to, and list-row-action renders
// nothing when ActionHref is empty.
func AdminRows(posts []Post) []Row {
	rows := make([]Row, 0, len(posts))
	for _, p := range posts {
		row := Row{
			Href: fmt.Sprintf("/admin/posts/%d/edit", p.ID),
			Main: p.Title,
			Sub:  DraftLine(p.UpdatedAt),
		}
		if p.Published {
			row.Sub = PublishedLine(p.CreatedAt)
			row.ActionHref = fmt.Sprintf("/posts/%d", p.ID)
			row.ActionLabel = "View"
			row.ActionAria = "View " + p.Title
		}
		rows = append(rows, row)
	}
	return rows
}

// Paragraphs splits a plain-text body on blank lines. Bodies are
// author-supplied plain text and stay that way: each paragraph is
// rendered as an ordinary {{.}} value, escaped by html/template, never
// template.HTML.
func Paragraphs(body string) []string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	var out []string
	for _, chunk := range strings.Split(body, "\n\n") {
		if s := strings.TrimSpace(chunk); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// BuildPagination builds the page strip for a list of total items at a
// 1-based page number. Show is false at or below one page, and the page
// template guards the partial with it.
func BuildPagination(base, q, status string, page, total int) Pagination {
	p := Pagination{Show: total > PageSize}
	if !p.Show {
		return p
	}
	pages := (total + PageSize - 1) / PageSize
	if page < 1 {
		page = 1
	}
	if page > pages {
		page = pages
	}

	// Built by hand rather than with url.Values.Encode, which sorts its
	// keys and would emit page before q.
	href := func(n int) string {
		var params []string
		if q != "" {
			params = append(params, "q="+url.QueryEscape(q))
		}
		if status != "" {
			params = append(params, "status="+status)
		}
		params = append(params, "page="+strconv.Itoa(n))
		return base + "?" + strings.Join(params, "&")
	}

	items := []PageItem{{Label: "Previous", Disabled: true}}
	if page > 1 {
		items[0] = PageItem{Label: "Previous", Href: href(page - 1)}
	}
	for _, n := range pageNumbers(page, pages) {
		switch {
		case n == 0:
			items = append(items, PageItem{Gap: true})
		case n == page:
			items = append(items, PageItem{Label: strconv.Itoa(n), Current: true})
		default:
			items = append(items, PageItem{Label: strconv.Itoa(n), Href: href(n)})
		}
	}
	next := PageItem{Label: "Next", Disabled: true}
	if page < pages {
		next = PageItem{Label: "Next", Href: href(page + 1)}
	}
	p.Items = append(items, next)
	return p
}

// pageNumbers returns the page numbers to render, with 0 standing for a
// gap. Up to seven pages every number renders; beyond that it is
// first · gap · current±1 · gap · last. A gap needs 71 posts to appear,
// so this app builds it correctly and never renders it — the library's
// own fixtures cover that item kind.
func pageNumbers(page, pages int) []int {
	if pages <= 7 {
		out := make([]int, 0, pages)
		for n := 1; n <= pages; n++ {
			out = append(out, n)
		}
		return out
	}
	want := map[int]bool{1: true, pages: true, page: true}
	if page-1 >= 1 {
		want[page-1] = true
	}
	if page+1 <= pages {
		want[page+1] = true
	}
	var out []int
	prev := 0
	for n := 1; n <= pages; n++ {
		if !want[n] {
			continue
		}
		if prev != 0 && n != prev+1 {
			out = append(out, 0)
		}
		out = append(out, n)
		prev = n
	}
	return out
}
