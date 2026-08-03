// Package generate's action emitter (this file) turns a validated
// rastrillo.Resource into the seven action files a manifest owns:
// gen/actions/<route>/{index.GET,index.POST,new.GET}.go and
// gen/actions/<route>/[id]/{index.GET,edit.GET,edit-basics.POST}.go,
// plus [id]/edit-advanced.POST.go when the resource declares
// Form.Advanced. Each file is written straight into gen/actions/ —
// unlike a hand action under actions/, it never passes through
// Discover/Rewrite, so it carries no `//go:build` constraint and
// compiles as a normal package from the moment it's written (this is
// what "generated actions go straight into gen/actions/, compiled
// normally" means in the task brief). A hand-written actions/<same
// path> file present under appRoot skips that one file — its computed
// gen/ path is reported in skipped, mirroring EmitTemplates.
//
// # The Ctx.Render seam
//
// Every generated action hands its page to the app's template tree
// through ctx.Render (rastrillo.Ctx's optional RenderFunc field — see
// ctx.go). Generated code cannot call an app-private helper like the
// blog's own blog.Render, so the seam is the one exported hook: the
// app's ctx factory sets Ctx.Render (e.g. &rastrillo.Ctx{DB: db,
// Render: blog.Render}), and every generated action nil-checks it
// before use, answering a logged 500 rather than a nil-pointer panic
// when an app forgets to wire it.
//
// # The page-name contract (pinned here; the blog adoption task must
// match it)
//
// A generated action calls ctx.Render(ctx, w, page, status, data) with
// page always one of exactly three names, independent of which of the
// seven files is calling:
//
//	"<resource.Name>/list"  — index.GET only
//	"<resource.Name>/show"  — [id]/index.GET only
//	"<resource.Name>/form"  — new.GET, [id]/edit.GET, and every 400
//	                          re-render (index.POST, edit-basics.POST,
//	                          edit-advanced.POST all share it, since
//	                          form.html's IsNew flag is what tells the
//	                          New state from the Edit state, not a
//	                          separate template)
//
// This mirrors the templates' own directory shape (gen/templates/
// <name>/{list,show,form}.html minus the .html suffix) rather than
// blog's buildPages, which keys its page map by bare basename only
// ("admin_list", "admin_edit", ...) — a bare "list"/"show"/"form" key
// would collide across every resource a manifest declares. An app
// wiring Render for generated resources must therefore key its own
// page map by "<name>/<file>" (e.g. by walking gen/templates and using
// each file's path relative to that root, minus ".html"), not by
// basename alone.
//
// # Behavior pinned here (encoded in the goldens; see actions_test.go)
//
//   - index.GET: parses q (only when List.Search is set — and only
//     really used by the store when there is at least one eligible
//     text/textarea List column; see searchColumns), one query param
//     per List.Filter entry (named by its sqlName, e.g. ?title=), and
//     page (default 1). Builds Rows from List.Columns[0] (Main) and,
//     if declared, List.Columns[1] (Sub) — Money formatted as a dollar
//     string via formatCents, everything else the raw column value.
//     Pagination has no gap-collapsing (v1 simplification: every page
//     number renders; see the report for the tradeoff).
//   - index.POST (create): parses every Form.Basics + Form.Advanced
//     field; a Money field's value is parsed as decimal dollars via
//     parseCents, which rejects more than two decimal places. Any
//     parse failure re-renders "<name>/form" (IsNew: true) at 400 with
//     the field's message in Errors and every field's raw submitted
//     text preserved in Fields — never reformatted, so a typo stays
//     exactly as typed for correction. Success stamps both timestamps
//     with the same UTC RFC3339 "now" and redirects 303 to Show.
//   - [id]/index.GET (show): Get-or-404, then every declared field
//     (columns(r) order) into Fields, Money formatted.
//   - new.GET / [id]/edit.GET: render "<name>/form" with zero values
//     (New) or the record's current values (Edit, Get-or-404 first).
//   - [id]/edit-basics.POST / [id]/edit-advanced.POST: Get-or-404 (to
//     answer 404 before ever attempting an UPDATE against a missing
//     row, and — only when this field group has a Money field — to
//     source the OTHER group's current values for the 400 re-render),
//     then parse and update only that field group. A Money parse
//     failure re-renders "<name>/form" (IsNew: false) at 400 with
//     Fields seeded from the fetched record and overridden with this
//     group's just-submitted raw text. A field group with no Money
//     field at all has no validation branch to speak of — generation
//     time already knows no parse can fail, so none is emitted (the
//     same "bake what the manifest already decided" discipline
//     templates.go uses for the Advanced-form and Search gates).
//
// No server-side required-field validation exists anywhere in this
// slice (a deliberate v1 rule) — an empty Money field parses to zero
// cents, never an error.
package generate

import (
	"bufio"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"

	"github.com/carlosframework/rastrillo"
)

// actionSpec is one of the (up to) seven files EmitActions considers
// for a resource.
type actionSpec struct {
	dir    string // slash-separated, relative to actions/, e.g. "admin/notes" or "admin/notes/[id]"
	name   string // filename stem: "index", "new", "edit", "edit-basics", "edit-advanced"
	method string
	build  func(rastrillo.Resource, string) string // (r, module) -> unformatted Go source
}

// EmitActions writes gen/actions/<route path>/... for r: index.GET,
// index.POST, new.GET, [id]/index.GET, [id]/edit.GET,
// [id]/edit-basics.POST, [id]/edit-advanced.POST (last one only when
// r.Form.Advanced is non-empty). A hand-written actions/<same path>
// file already present under appRoot skips generating that one file —
// its computed gen/ path is reported in skipped, and nothing is
// written or touched there. Returns the written/skipped gen/ paths.
func EmitActions(appRoot, genDir string, r rastrillo.Resource) (written, skipped []string, err error) {
	module, err := actionsModulePath(appRoot)
	if err != nil {
		return nil, nil, err
	}

	resDir := strings.TrimPrefix(r.Route, "/")
	idDir := resDir + "/[id]"

	specs := []actionSpec{
		{resDir, "index", "GET", actionIndexGET},
		{resDir, "index", "POST", actionIndexPOST},
		{resDir, "new", "GET", actionNewGET},
		{idDir, "index", "GET", actionShowGET},
		{idDir, "edit", "GET", actionEditGET},
		{idDir, "edit-basics", "POST", actionEditBasicsPOST},
	}
	if len(r.Form.Advanced) > 0 {
		specs = append(specs, actionSpec{idDir, "edit-advanced", "POST", actionEditAdvancedPOST})
	}

	for _, s := range specs {
		base := s.name + "." + s.method + ".go"
		relSource := s.dir + "/" + base
		genPath := filepath.Join(genDir, "actions", filepath.FromSlash(genDirFor(s.dir, s.name, s.method)), base)
		handPath := filepath.Join(appRoot, "actions", filepath.FromSlash(relSource))

		if _, statErr := os.Stat(handPath); statErr == nil {
			skipped = append(skipped, genPath)
			continue
		} else if !os.IsNotExist(statErr) {
			return nil, nil, fmt.Errorf("%s: %w", relSource, statErr)
		}

		pkg := packageNameFor(relSource)
		src := "// Code generated by rastrillo generate. DO NOT EDIT.\n\npackage " + pkg + "\n\n" + s.build(r, module)
		formatted, ferr := format.Source([]byte(src))
		if ferr != nil {
			return nil, nil, fmt.Errorf("%s: %w", relSource, ferr)
		}
		if err := writeFileIfChanged(genPath, formatted); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", relSource, err)
		}
		written = append(written, genPath)
	}

	return written, skipped, nil
}

// actionsModulePath reads the module directive from appRoot's go.mod.
// Hand-rolled rather than shared across packages, matching this
// codebase's own precedent (internal/manifest/goeval.go's
// readModulePath, cmd/rastrillo/modpath.go's modulePath): the module
// line is one fixed shape, and each package that needs it keeps its
// own unexported copy.
func actionsModulePath(appRoot string) (string, error) {
	f, err := os.Open(filepath.Join(appRoot, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("go.mod: no module directive found")
}

// storeImport returns the store package's import alias and path for r:
// alias equals the package name sqlc.yaml told sqlc to generate
// (<name>store — see store.go's sqlcYAML), written explicitly rather
// than relying on the compiler inferring it from the package clause,
// matching Router()'s own explicit-alias style in generate.go.
func storeImport(r rastrillo.Resource, module string) (alias, path string) {
	return r.Name + "store", module + "/gen/store/" + r.Name
}

// mainSubColumns returns the columns index.GET's Main/Sub and show's
// Title derive from: List.Columns[0] and, if declared, List.Columns[1].
// Falls back to columns(r)'s first entry when the resource declares no
// List columns at all (Validate only requires at least one List column
// OR Form field, not specifically the former) — a real but unlikely
// manifest shape, handled rather than left to panic.
func mainSubColumns(r rastrillo.Resource) (main column, sub column, hasSub bool) {
	if len(r.List.Columns) == 0 {
		if cols := columns(r); len(cols) > 0 {
			return cols[0], column{}, false
		}
		return column{}, column{}, false
	}
	c0 := r.List.Columns[0]
	main = column{Name: c0.Field, SQL: sqlName(c0.Field), Kind: c0.Kind}
	if len(r.List.Columns) > 1 {
		c1 := r.List.Columns[1]
		sub = column{Name: c1.Field, SQL: sqlName(c1.Field), Kind: c1.Kind}
		hasSub = true
	}
	return main, sub, hasSub
}

// fieldExpr renders the Go expression that reads c's current value off
// a record variable named varName: a Money column reads through
// formatCents (every template-facing value is a display string,
// never math the template does itself — the same discipline
// templates.go documents for show.html/form.html), everything else is
// the raw sqlc-generated field.
func fieldExpr(varName string, c column) string {
	if c.Kind == rastrillo.Money {
		return fmt.Sprintf("formatCents(%s.%s)", varName, c.Name)
	}
	return varName + "." + c.Name
}

// fieldsMapLiteral renders a `map[string]string{...}` literal for
// every column in columns(r), each keyed by its declared name and
// valued by fieldExpr(varName, c) — the shape show.html and the Edit
// half of form.html both read as Fields.
func fieldsMapLiteral(r rastrillo.Resource, varName string) string {
	var b strings.Builder
	b.WriteString("map[string]string{\n")
	for _, c := range columns(r) {
		fmt.Fprintf(&b, "%q: %s,\n", c.Name, fieldExpr(varName, c))
	}
	b.WriteString("}")
	return b.String()
}

// zeroFieldsMapLiteral renders the New form's Fields: every declared
// field name mapped to "".
func zeroFieldsMapLiteral(r rastrillo.Resource) string {
	var b strings.Builder
	b.WriteString("map[string]string{\n")
	for _, c := range columns(r) {
		fmt.Fprintf(&b, "%q: \"\",\n", c.Name)
	}
	b.WriteString("}")
	return b.String()
}

// filterVar is one List.Filter entry's generated shape: a query-string
// key (its sqlName) and a local variable name ("filter" + the declared
// name — reconstructing the declared name exactly, since sqlName then
// pascalCase round-trips any identPattern-valid identifier).
type filterVar struct {
	Field string // declared name, e.g. "Title"
	Query string // sqlName(Field), the URL query key, e.g. "title"
	Var   string // "filterTitle"
	Param string // the store Params field name, "Filter"+Field
}

func filterVars(r rastrillo.Resource) []filterVar {
	var out []filterVar
	for _, f := range r.List.Filter {
		out = append(out, filterVar{
			Field: f,
			Query: sqlName(f),
			Var:   "filter" + f,
			Param: "Filter" + f,
		})
	}
	return out
}

// ── Shared boilerplate, identical across all seven files for a given
// resource (only the "<name>: " log prefix varies) ─────────────────

// helperFuncs renders the fail/render/parseID/formatCents/parseCents
// helpers every generated action carries. Including all five
// unconditionally in every file (rather than computing per-file which
// are actually called) keeps the emitter's per-file logic simple and
// costs nothing real: an unused top-level func is valid Go, and each
// helper's own body is what pulls in "fmt"/"strconv"/"strings" — never
// a needless import.
func helperFuncs(name string) string {
	return fmt.Sprintf(`
// fail logs through Ctx.Logger (when set) and answers a plain 500.
func fail(ctx *rastrillo.Ctx, w http.ResponseWriter, what string, err error) {
	if ctx.Logger != nil {
		ctx.Logger.Error(%q+what, "err", err)
	}
	http.Error(w, "Something went wrong.", http.StatusInternalServerError)
}

// render hands data to the app's template tree through ctx.Render (see
// rastrillo.Ctx's Render field) — a 500 with a clear log line stands in
// for a template an app forgot to wire, rather than a nil-pointer panic.
func render(ctx *rastrillo.Ctx, w http.ResponseWriter, page string, status int, data any) {
	if ctx.Render == nil {
		if ctx.Logger != nil {
			ctx.Logger.Error(%q + "Ctx.Render is nil; the app's ctx factory must set it")
		}
		http.Error(w, "Something went wrong.", http.StatusInternalServerError)
		return
	}
	ctx.Render(ctx, w, page, status, data)
}

// parseID reads the {id} path value. A non-numeric id is a URL that
// was never ours, so the caller answers 404 rather than 400.
func parseID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, false
	}
	return id, true
}

// formatCents renders cents as a dollar string — the only money
// formatting a generated template ever sees; a template never does
// money math itself.
func formatCents(cents int64) string {
	return fmt.Sprintf("$%%d.%%02d", cents/100, cents%%100)
}

// parseCents parses a decimal-dollars string (e.g. "12.34") into
// cents, rejecting more than two decimal places. An empty string
// parses to zero cents, not an error — v1 has no server-side
// required-field validation.
func parseCents(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}
	whole, frac, hasFrac := strings.Cut(s, ".")
	if hasFrac && len(frac) > 2 {
		return 0, fmt.Errorf("enter a dollar amount with at most 2 decimal places")
	}
	for len(frac) < 2 {
		frac += "0"
	}
	if whole == "" {
		whole = "0"
	}
	wholeN, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("enter a valid dollar amount")
	}
	fracN, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("enter a valid dollar amount")
	}
	cents := wholeN*100 + fracN
	if neg {
		cents = -cents
	}
	return cents, nil
}
`, name+": ", name+": ")
}

const listViewTypes = `
type listView struct {
	Empty      bool
	Query      string
	Carry      [][2]string
	Rows       []listRow
	Pagination listPagination
}

type listRow struct {
	Href string
	Main string
	Sub  string
}

type listPagination struct {
	Show  bool
	Items []listPageItem
}

type listPageItem struct {
	Label    string
	Href     string
	Current  bool
	Disabled bool
	Gap      bool
}
`

const showViewType = `
type showView struct {
	Title    string
	EditHref string
	Fields   map[string]string
}
`

const formViewType = `
type formView struct {
	IsNew          bool
	Fields         map[string]string
	Errors         map[string]string
	BasicsAction   string
	AdvancedAction string
}
`

// ── index.GET (List) ────────────────────────────────────────────────

func actionIndexGET(r rastrillo.Resource, module string) string {
	alias, path := storeImport(r, module)
	fvars := filterVars(r)
	searchDeclared := r.List.Search
	searchParam := r.List.Search && len(searchColumns(r)) > 0
	main, sub, hasSub := mainSubColumns(r)

	var b strings.Builder
	b.WriteString("import (\n")
	b.WriteString("\t\"fmt\"\n")
	b.WriteString("\t\"net/http\"\n")
	b.WriteString("\t\"net/url\"\n")
	b.WriteString("\t\"strconv\"\n")
	b.WriteString("\t\"strings\"\n\n")
	b.WriteString("\t\"github.com/carlosframework/rastrillo\"\n")
	fmt.Fprintf(&b, "\t%s %q\n", alias, path)
	b.WriteString(")\n")

	fmt.Fprintf(&b, "\n// Handle is GET %s.\n", r.Route)
	b.WriteString("func Handle(ctx *rastrillo.Ctx, w http.ResponseWriter, r *http.Request) {\n")

	searchExpr := `""`
	if searchDeclared {
		b.WriteString("search := strings.TrimSpace(r.URL.Query().Get(\"q\"))\n")
		searchExpr = "search"
	}
	for _, fv := range fvars {
		fmt.Fprintf(&b, "%s := r.URL.Query().Get(%q)\n", fv.Var, fv.Query)
	}

	b.WriteString(`
page := 1
if v := r.URL.Query().Get("page"); v != "" {
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		page = n
	}
}
const pageSize = 10
offset := (page - 1) * pageSize
`)

	b.WriteString("\nvar carry [][2]string\n")
	for _, fv := range fvars {
		fmt.Fprintf(&b, "if %s != \"\" {\n\tcarry = append(carry, [2]string{%q, %s})\n}\n", fv.Var, fv.Query, fv.Var)
	}

	fmt.Fprintf(&b, "\nstore := %s.New(ctx.DB)\n", alias)
	plural := pascalCase(r.Name)

	// CountX has no tail clause the way ListX's LIMIT/OFFSET always
	// gives it one — a resource with neither a search box nor a
	// declared Filter has a Count query with zero bind parameters, and
	// sqlc's own convention (verified against a real `sqlc generate`
	// run) is to drop the Params argument entirely in that case, not
	// generate an empty struct type. Match that exactly, or the
	// generated call would pass an argument the real method doesn't
	// accept.
	countHasParams := searchParam || len(fvars) > 0
	if countHasParams {
		b.WriteString("total, err := store.Count" + plural + "(r.Context(), " + alias + ".Count" + plural + "Params{\n")
		if searchParam {
			b.WriteString("Search: " + searchExpr + ",\n")
		}
		for _, fv := range fvars {
			fmt.Fprintf(&b, "%s: %s,\n", fv.Param, fv.Var)
		}
		b.WriteString("})\n")
	} else {
		b.WriteString("total, err := store.Count" + plural + "(r.Context())\n")
	}
	fmt.Fprintf(&b, "if err != nil {\n\tfail(ctx, w, %q, err)\n\treturn\n}\n", "counting "+r.Name)

	b.WriteString("rows, err := store.List" + plural + "(r.Context(), " + alias + ".List" + plural + "Params{\n")
	if searchParam {
		b.WriteString("Search: " + searchExpr + ",\n")
	}
	for _, fv := range fvars {
		fmt.Fprintf(&b, "%s: %s,\n", fv.Param, fv.Var)
	}
	b.WriteString("PageOffset: int64(offset),\nPageLimit: pageSize,\n")
	b.WriteString("})\n")
	fmt.Fprintf(&b, "if err != nil {\n\tfail(ctx, w, %q, err)\n\treturn\n}\n", "loading "+r.Name)

	fmt.Fprintf(&b, `
items := make([]listRow, 0, len(rows))
for _, n := range rows {
	items = append(items, listRow{
		Href: fmt.Sprintf(%q, n.ID),
		Main: %s,
		Sub:  %s,
	})
}
`, r.Route+"/%d", fieldExpr("n", main), subExpr(sub, hasSub))

	b.WriteString(`
totalPages := (int(total) + pageSize - 1) / pageSize
show := int(total) > pageSize
var pageItems []listPageItem
if show {
	prev := listPageItem{Label: "Previous", Disabled: true}
	if page > 1 {
		prev = listPageItem{Label: "Previous", Href: href(` + searchExprForHref(searchDeclared) + `, carry, page-1)}
	}
	pageItems = append(pageItems, prev)
	for n := 1; n <= totalPages; n++ {
		if n == page {
			pageItems = append(pageItems, listPageItem{Label: strconv.Itoa(n), Current: true})
		} else {
			pageItems = append(pageItems, listPageItem{Label: strconv.Itoa(n), Href: href(` + searchExprForHref(searchDeclared) + `, carry, n)})
		}
	}
	next := listPageItem{Label: "Next", Disabled: true}
	if page < totalPages {
		next = listPageItem{Label: "Next", Href: href(` + searchExprForHref(searchDeclared) + `, carry, page+1)}
	}
	pageItems = append(pageItems, next)
}
`)

	fmt.Fprintf(&b, `
render(ctx, w, %q, http.StatusOK, listView{
	Empty:      total == 0,
	Query:      %s,
	Carry:      carry,
	Rows:       items,
	Pagination: listPagination{Show: show, Items: pageItems},
})
}
`, r.Name+"/list", searchExpr)

	fmt.Fprintf(&b, `
// href builds one list/pagination link, preserving the current search
// and filter values and setting page.
func href(search string, carry [][2]string, page int) string {
	var params []string
	if search != "" {
		params = append(params, "q="+url.QueryEscape(search))
	}
	for _, kv := range carry {
		params = append(params, kv[0]+"="+url.QueryEscape(kv[1]))
	}
	params = append(params, "page="+strconv.Itoa(page))
	return %q + "?" + strings.Join(params, "&")
}
`, r.Route)

	b.WriteString(listViewTypes)
	b.WriteString(helperFuncs(r.Name))
	return b.String()
}

// subExpr renders the Sub row expression: an empty literal when the
// resource declares no second List column.
func subExpr(sub column, hasSub bool) string {
	if !hasSub {
		return `""`
	}
	return fieldExpr("n", sub)
}

// searchExprForHref is the argument href() is called with at each of
// its three call sites: the live "search" variable when List.Search
// declares a search box at all (independent of whether the store ends
// up honoring it — see searchParam), or a literal "" when it doesn't,
// so no unused variable is ever emitted.
func searchExprForHref(searchDeclared bool) string {
	if searchDeclared {
		return "search"
	}
	return `""`
}

// ── index.POST (Create) ─────────────────────────────────────────────

func actionIndexPOST(r rastrillo.Resource, module string) string {
	alias, path := storeImport(r, module)
	singular := singularPascal(r.Name)

	byName := map[string]parsedField{}
	var decls []string
	var order []rastrillo.Field
	order = append(order, r.Form.Basics...)
	order = append(order, r.Form.Advanced...)
	for _, f := range order {
		pf := parseField(f)
		byName[f.Name] = pf
		decls = append(decls, pf.Decls)
	}

	var errChecks []string
	for _, f := range order {
		if ec := byName[f.Name].ErrCheck; ec != "" {
			errChecks = append(errChecks, ec)
		}
	}

	var paramLines []string
	var rawFieldLines []string
	for _, c := range columns(r) {
		if pf, ok := byName[c.Name]; ok {
			paramLines = append(paramLines, pf.ParamLine)
			rawFieldLines = append(rawFieldLines, fmt.Sprintf("%q: %s,\n", c.Name, pf.RawExpr))
			continue
		}
		// A List-only column with no matching Form field: Create has
		// no submitted value for it, so it gets the zero value. (A
		// well-formed manifest gives every List column a Form field
		// too; this is a fallback, not the expected shape.)
		zero := `""`
		if c.Kind == rastrillo.Money {
			zero = "0"
		}
		paramLines = append(paramLines, fmt.Sprintf("%s: %s,\n", c.Name, zero))
		rawFieldLines = append(rawFieldLines, fmt.Sprintf("%q: \"\",\n", c.Name))
	}

	var b strings.Builder
	b.WriteString("import (\n")
	b.WriteString("\t\"fmt\"\n")
	b.WriteString("\t\"net/http\"\n")
	b.WriteString("\t\"strconv\"\n")
	b.WriteString("\t\"strings\"\n")
	b.WriteString("\t\"time\"\n\n")
	b.WriteString("\t\"github.com/carlosframework/rastrillo\"\n")
	fmt.Fprintf(&b, "\t%s %q\n", alias, path)
	b.WriteString(")\n")

	fmt.Fprintf(&b, "\n// Handle is POST %s.\n", r.Route)
	b.WriteString(`func Handle(ctx *rastrillo.Ctx, w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request.", http.StatusBadRequest)
		return
	}

`)
	for _, d := range decls {
		b.WriteString(d)
	}

	if len(errChecks) > 0 {
		b.WriteString("\nerrs := map[string]string{}\n")
		for _, ec := range errChecks {
			b.WriteString(ec)
		}
		b.WriteString("\nif len(errs) > 0 {\n")
		fmt.Fprintf(&b, "render(ctx, w, %q, http.StatusBadRequest, formView{\n", r.Name+"/form")
		b.WriteString("IsNew: true,\nFields: map[string]string{\n")
		for _, l := range rawFieldLines {
			b.WriteString(l)
		}
		b.WriteString("},\nErrors: errs,\n})\nreturn\n}\n")
	}

	b.WriteString("\nnow := time.Now().UTC().Format(time.RFC3339)\n")
	fmt.Fprintf(&b, "store := %s.New(ctx.DB)\n", alias)
	fmt.Fprintf(&b, "id, err := store.Create%s(r.Context(), %s.Create%sParams{\n", singular, alias, singular)
	for _, l := range paramLines {
		b.WriteString(l)
	}
	b.WriteString("Now: now,\n")
	b.WriteString("})\n")
	fmt.Fprintf(&b, "if err != nil {\n\tfail(ctx, w, %q, err)\n\treturn\n}\n", "creating "+r.Name)
	fmt.Fprintf(&b, "http.Redirect(w, r, fmt.Sprintf(%q, id), http.StatusSeeOther)\n}\n", r.Route+"/%d")

	b.WriteString(formViewType)
	b.WriteString(helperFuncs(r.Name))
	return b.String()
}

// ── new.GET ──────────────────────────────────────────────────────────

func actionNewGET(r rastrillo.Resource, module string) string {
	var b strings.Builder
	b.WriteString("import (\n")
	b.WriteString("\t\"fmt\"\n")
	b.WriteString("\t\"net/http\"\n")
	b.WriteString("\t\"strconv\"\n")
	b.WriteString("\t\"strings\"\n\n")
	b.WriteString("\t\"github.com/carlosframework/rastrillo\"\n")
	b.WriteString(")\n")

	fmt.Fprintf(&b, "\n// Handle is GET %s/new.\n", r.Route)
	b.WriteString("func Handle(ctx *rastrillo.Ctx, w http.ResponseWriter, r *http.Request) {\n")
	fmt.Fprintf(&b, "render(ctx, w, %q, http.StatusOK, formView{\n", r.Name+"/form")
	b.WriteString("IsNew: true,\nFields: " + zeroFieldsMapLiteral(r) + ",\n")
	b.WriteString("})\n}\n")

	b.WriteString(formViewType)
	b.WriteString(helperFuncs(r.Name))
	return b.String()
}

// ── [id]/index.GET (Show) ───────────────────────────────────────────

func actionShowGET(r rastrillo.Resource, module string) string {
	alias, path := storeImport(r, module)
	singular := singularPascal(r.Name)
	main, _, _ := mainSubColumns(r)

	var b strings.Builder
	b.WriteString("import (\n")
	b.WriteString("\t\"database/sql\"\n")
	b.WriteString("\t\"errors\"\n")
	b.WriteString("\t\"fmt\"\n")
	b.WriteString("\t\"net/http\"\n")
	b.WriteString("\t\"strconv\"\n")
	b.WriteString("\t\"strings\"\n\n")
	b.WriteString("\t\"github.com/carlosframework/rastrillo\"\n")
	fmt.Fprintf(&b, "\t%s %q\n", alias, path)
	b.WriteString(")\n")

	fmt.Fprintf(&b, "\n// Handle is GET %s/{id}.\n", r.Route)
	b.WriteString(`func Handle(ctx *rastrillo.Ctx, w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
`)
	fmt.Fprintf(&b, "store := %s.New(ctx.DB)\n", alias)
	fmt.Fprintf(&b, "n, err := store.Get%s(r.Context(), id)\n", singular)
	b.WriteString(`if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
`)
	fmt.Fprintf(&b, "if err != nil {\n\tfail(ctx, w, %q, err)\n\treturn\n}\n", "loading "+r.Name)

	fmt.Fprintf(&b, `
render(ctx, w, %q, http.StatusOK, showView{
	Title:    %s,
	EditHref: fmt.Sprintf(%q, id),
	Fields:   %s,
})
}
`, r.Name+"/show", fieldExpr("n", main), r.Route+"/%d/edit", fieldsMapLiteral(r, "n"))

	b.WriteString(showViewType)
	b.WriteString(helperFuncs(r.Name))
	return b.String()
}

// ── [id]/edit.GET ────────────────────────────────────────────────────

func actionEditGET(r rastrillo.Resource, module string) string {
	alias, path := storeImport(r, module)
	singular := singularPascal(r.Name)
	hasAdvanced := len(r.Form.Advanced) > 0

	var b strings.Builder
	b.WriteString("import (\n")
	b.WriteString("\t\"database/sql\"\n")
	b.WriteString("\t\"errors\"\n")
	b.WriteString("\t\"fmt\"\n")
	b.WriteString("\t\"net/http\"\n")
	b.WriteString("\t\"strconv\"\n")
	b.WriteString("\t\"strings\"\n\n")
	b.WriteString("\t\"github.com/carlosframework/rastrillo\"\n")
	fmt.Fprintf(&b, "\t%s %q\n", alias, path)
	b.WriteString(")\n")

	fmt.Fprintf(&b, "\n// Handle is GET %s/{id}/edit.\n", r.Route)
	b.WriteString(`func Handle(ctx *rastrillo.Ctx, w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
`)
	fmt.Fprintf(&b, "store := %s.New(ctx.DB)\n", alias)
	fmt.Fprintf(&b, "n, err := store.Get%s(r.Context(), id)\n", singular)
	b.WriteString(`if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
`)
	fmt.Fprintf(&b, "if err != nil {\n\tfail(ctx, w, %q, err)\n\treturn\n}\n", "loading "+r.Name)

	fmt.Fprintf(&b, `
render(ctx, w, %q, http.StatusOK, formView{
	IsNew:  false,
	Fields: %s,
	BasicsAction: fmt.Sprintf(%q, id),
`, r.Name+"/form", fieldsMapLiteral(r, "n"), r.Route+"/%d/edit-basics")
	if hasAdvanced {
		fmt.Fprintf(&b, "AdvancedAction: fmt.Sprintf(%q, id),\n", r.Route+"/%d/edit-advanced")
	}
	b.WriteString("})\n}\n")

	b.WriteString(formViewType)
	b.WriteString(helperFuncs(r.Name))
	return b.String()
}

// ── [id]/edit-basics.POST and [id]/edit-advanced.POST ──────────────

// updatePOST builds edit-basics.POST or edit-advanced.POST: they are
// the same shape (Get-or-404, parse this group's fields, update,
// redirect — or, only when this group declares a Money field, a
// validation branch), parameterized on which field group and which
// sqlc Update query own it.
func updatePOST(r rastrillo.Resource, module string, group []rastrillo.Field, groupName string) string {
	alias, path := storeImport(r, module)
	singular := singularPascal(r.Name)
	hasAdvanced := len(r.Form.Advanced) > 0

	byName := map[string]parsedField{}
	var decls []string
	for _, f := range group {
		pf := parseField(f)
		byName[f.Name] = pf
		decls = append(decls, pf.Decls)
	}
	var errChecks []string
	for _, f := range group {
		if ec := byName[f.Name].ErrCheck; ec != "" {
			errChecks = append(errChecks, ec)
		}
	}
	groupHasMoney := len(errChecks) > 0

	var paramLines []string
	for _, f := range group {
		paramLines = append(paramLines, byName[f.Name].ParamLine)
	}

	var b strings.Builder
	b.WriteString("import (\n")
	b.WriteString("\t\"database/sql\"\n")
	b.WriteString("\t\"errors\"\n")
	b.WriteString("\t\"fmt\"\n")
	b.WriteString("\t\"net/http\"\n")
	b.WriteString("\t\"strconv\"\n")
	b.WriteString("\t\"strings\"\n")
	b.WriteString("\t\"time\"\n\n")
	b.WriteString("\t\"github.com/carlosframework/rastrillo\"\n")
	fmt.Fprintf(&b, "\t%s %q\n", alias, path)
	b.WriteString(")\n")

	fmt.Fprintf(&b, "\n// Handle is POST %s/{id}/edit-%s.\n", r.Route, strings.ToLower(groupName))
	b.WriteString(`func Handle(ctx *rastrillo.Ctx, w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request.", http.StatusBadRequest)
		return
	}
	id, ok := parseID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
`)
	fmt.Fprintf(&b, "store := %s.New(ctx.DB)\n", alias)

	if groupHasMoney {
		fmt.Fprintf(&b, "n, err := store.Get%s(r.Context(), id)\n", singular)
	} else {
		fmt.Fprintf(&b, "_, err := store.Get%s(r.Context(), id)\n", singular)
	}
	b.WriteString(`if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
`)
	fmt.Fprintf(&b, "if err != nil {\n\tfail(ctx, w, %q, err)\n\treturn\n}\n\n", "loading "+r.Name)

	for _, d := range decls {
		b.WriteString(d)
	}

	if groupHasMoney {
		b.WriteString("\nerrs := map[string]string{}\n")
		for _, ec := range errChecks {
			b.WriteString(ec)
		}
		b.WriteString("\nif len(errs) > 0 {\n")
		b.WriteString("fields := " + fieldsMapLiteral(r, "n") + "\n")
		for _, f := range group {
			fmt.Fprintf(&b, "fields[%q] = %s\n", f.Name, byName[f.Name].RawExpr)
		}
		fmt.Fprintf(&b, "render(ctx, w, %q, http.StatusBadRequest, formView{\n", r.Name+"/form")
		b.WriteString("IsNew: false,\nFields: fields,\nErrors: errs,\n")
		fmt.Fprintf(&b, "BasicsAction: fmt.Sprintf(%q, id),\n", r.Route+"/%d/edit-basics")
		if hasAdvanced {
			fmt.Fprintf(&b, "AdvancedAction: fmt.Sprintf(%q, id),\n", r.Route+"/%d/edit-advanced")
		}
		b.WriteString("})\nreturn\n}\n")
	}

	b.WriteString("\nnow := time.Now().UTC().Format(time.RFC3339)\n")
	fmt.Fprintf(&b, "if err := store.Update%s%s(r.Context(), %s.Update%s%sParams{\n",
		singular, groupName, alias, singular, groupName)
	for _, l := range paramLines {
		b.WriteString(l)
	}
	b.WriteString("Now: now,\nID: id,\n")
	b.WriteString("}); err != nil {\n")
	fmt.Fprintf(&b, "fail(ctx, w, %q, err)\nreturn\n}\n", "updating "+r.Name)
	fmt.Fprintf(&b, "http.Redirect(w, r, fmt.Sprintf(%q, id), http.StatusSeeOther)\n}\n", r.Route+"/%d")

	if groupHasMoney {
		b.WriteString(formViewType)
	}
	b.WriteString(helperFuncs(r.Name))
	return b.String()
}

func actionEditBasicsPOST(r rastrillo.Resource, module string) string {
	return updatePOST(r, module, r.Form.Basics, "Basics")
}

func actionEditAdvancedPOST(r rastrillo.Resource, module string) string {
	return updatePOST(r, module, r.Form.Advanced, "Advanced")
}

// ── field parsing shared by Create and both Update actions ─────────

// parsedField is one Basics/Advanced field's generated parsing code.
type parsedField struct {
	Decls     string // Go statements assigning its local variable(s)
	ParamLine string // e.g. "Price: vPrice,\n" — a store Params field
	RawExpr   string // the just-submitted value, for a 400 re-render's Fields override
	ErrCheck  string // "" or an `if errX != nil { errs["Price"] = ... }` block
}

func parseField(f rastrillo.Field) parsedField {
	v := "v" + f.Name
	switch f.Kind {
	case rastrillo.Textarea:
		return parsedField{
			Decls:     fmt.Sprintf("%s := r.PostFormValue(%q)\n", v, f.Name),
			ParamLine: fmt.Sprintf("%s: %s,\n", f.Name, v),
			RawExpr:   v,
		}
	case rastrillo.Money:
		raw := v + "Raw"
		errVar := "err" + f.Name
		return parsedField{
			Decls: fmt.Sprintf("%s := r.PostFormValue(%q)\n%s, %s := parseCents(%s)\n",
				raw, f.Name, v, errVar, raw),
			ParamLine: fmt.Sprintf("%s: %s,\n", f.Name, v),
			RawExpr:   raw,
			ErrCheck:  fmt.Sprintf("if %s != nil {\nerrs[%q] = %s.Error()\n}\n", errVar, f.Name, errVar),
		}
	default: // Text
		return parsedField{
			Decls:     fmt.Sprintf("%s := strings.TrimSpace(r.PostFormValue(%q))\n", v, f.Name),
			ParamLine: fmt.Sprintf("%s: %s,\n", f.Name, v),
			RawExpr:   v,
		}
	}
}
