package blog

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path"
	"reflect"
	"strings"

	blogassets "blog"
	"blog/gen/locales"

	"github.com/carlosframework/rastrillo"
	"github.com/carlosframework/rastrillo/ui"
)

// genPages is one template tree per generated screen, keyed
// "<resource>/<page>" (e.g. "posts/form") — the exact contract
// internal/generate's action emitter pins (see its package doc):
// generated actions call ctx.Render(ctx, w, "<resource>/<page>", ...),
// so Render (below) dispatches on whether page contains a "/" to tell
// a generated page from one of the app's own (which are keyed by bare
// basename — "admin_list", "index" — in the pages map above).
//
// Built once at process start, the same way pages is.
var genPages = buildGenPages()

// genLayoutHTML is layout.html's shape, restated for generated pages.
// It cannot be the same template: layout.html calls {{template
// "content" .}}, passing its data straight through, and every hand
// view model (HomeView, PostView, ...) carries its own Head field
// alongside its screen-specific fields so that works. A generated view
// model (formView, showView, ...) has no Head field at all — the
// action emitter doesn't know about the app's layout — so this layout
// instead takes a genPageData wrapper and passes only .Data to
// "content", keeping the generated view models exactly as the
// generator pins them.
//
// Task 11 (template-tree wiring) is expected to reconcile this
// duplication once generated pages start getting ejected into
// templates/pages/ alongside the hand ones.
const genLayoutHTML = `{{define "gen-layout"}}<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Head.Title}} · The blog</title>
<link rel="stylesheet" href="/static/tokens.css">
<link rel="stylesheet" href="/static/blog.css">
</head>
<body>
<div class="rst-page">
{{template "content" .Data}}
<footer class="blog-footer">
<a href="/">The blog</a>
<a href="/admin/posts">Posts admin</a>
</footer>
</div>
</body>
</html>
{{end}}`

// genPageData is genLayoutHTML's one data value: Head carries what the
// layout itself needs, Data is passed through to "content" untouched
// so a generated view model's fields (.IsNew, .Fields, ...) resolve
// exactly as internal/generate's templates.go emitted them.
type genPageData struct {
	Head Head
	Data any
}

// buildGenPages parses ui's partials and genLayoutHTML into a base
// tree, then clones that base once per generated screen — the same
// clone-per-file technique buildPages uses and for the same reason:
// every screen defines "content", so parsing them into one shared tree
// would let the last one clobber the rest.
//
// gen/templates holds one subdirectory per manifest resource (today,
// just posts/); this walks all of them so a second resource's
// templates are picked up without this file changing.
func buildGenPages() map[string]*template.Template {
	sub, err := fs.Sub(blogassets.GenTemplatesFS, "gen/templates")
	if err != nil {
		panic(err) // gen/templates is embedded in genassets.go; this cannot fail
	}

	base := template.New("").Funcs(ui.Funcs()).Funcs(template.FuncMap{"T": genT})
	base = template.Must(base.Parse(genLayoutHTML))
	base = template.Must(base.ParseFS(ui.Templates(), "*.html"))

	resDirs, err := fs.ReadDir(sub, ".")
	if err != nil {
		panic(err)
	}

	out := map[string]*template.Template{}
	for _, rd := range resDirs {
		if !rd.IsDir() {
			continue
		}
		files, err := fs.Glob(sub, rd.Name()+"/*.html")
		if err != nil {
			panic(err)
		}
		for _, f := range files {
			t := template.Must(base.Clone())
			t = template.Must(t.ParseFS(sub, f))
			key := rd.Name() + "/" + strings.TrimSuffix(path.Base(f), ".html")
			out[key] = t
		}
	}
	return out
}

// genT resolves a manifest catalog key against gen/locales.BaseCatalog
// — the app is monolingual today, so this is a direct lookup rather
// than the request-scoped rastrillo.T/Locales machinery (locale.go,
// localemw.go), which needs Options.Locales wired to install its
// middleware; adopting that is out of this task's scope. A missing key
// renders as the key itself, matching (*rastrillo.Locales).T's own
// fallback, so a typo shows up on the page instead of blanking a
// sentence.
func genT(key string) string {
	if v, ok := locales.BaseCatalog[key]; ok {
		return v
	}
	return key
}

// genHead computes the Head a generated page's layout needs. Generated
// view models carry no Head field (see genPageData's doc comment), so
// this is derived from the page name plus, for the form screen, a
// glance at the data's own IsNew field via reflection — the only way
// to read it generically, since a generated formView is a distinct,
// unexported type per action file (new.GET's, edit.GET's, and both
// POSTs' 400 re-renders each declare their own copy; see actions.go's
// formViewType) and none of them are reachable from this package.
func genHead(page string, data any) Head {
	switch page {
	case "posts/show":
		return Head{Title: "Post"}
	case "posts/form":
		if isNewForm(data) {
			return Head{Title: "New post"}
		}
		return Head{Title: "Edit post"}
	default: // "posts/list" and any future resource's list page
		return Head{Title: "Posts"}
	}
}

// isNewForm reads a generated formView-shaped value's IsNew field by
// reflection — see genHead's doc comment for why reflection, not a
// type assertion, is the only generic option here.
func isNewForm(data any) bool {
	v := reflect.ValueOf(data)
	if v.Kind() != reflect.Struct {
		return false
	}
	f := v.FieldByName("IsNew")
	return f.IsValid() && f.Kind() == reflect.Bool && f.Bool()
}

// renderGen is Render's generated-page half: look up page in genPages,
// execute "gen-layout" with data wrapped in genPageData, and write the
// result exactly the way Render's own hand-page half does (buffered
// first, so a template error is a clean 500 rather than half a page).
func renderGen(ctx *rastrillo.Ctx, w http.ResponseWriter, page string, status int, data any) {
	t, ok := genPages[page]
	if !ok {
		Fail(ctx, w, "rendering "+page, fmt.Errorf("no such generated page template"))
		return
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "gen-layout", genPageData{Head: genHead(page, data), Data: data}); err != nil {
		Fail(ctx, w, "rendering "+page, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	w.Write(buf.Bytes())
}
