package ui

import (
	"fmt"
	"html/template"

	"github.com/carlosframework/rastrillo"
)

// Funcs returns the helpers ui's partials call, for an app to register on
// its own template tree:
//
//	tmpl := template.Must(template.New("").Funcs(ui.Funcs()).
//	        ParseFS(ui.Templates(), "*.html"))
//
// dict and list are the map/slice builders that let a caller compose a
// partial's one data value inline, with no Go view-model type per
// combination. icon is the root package's vendored-Lucide lookup
// (rastrillo.Icon), so {{icon "search"}} resolves identically inside a
// vendored partial and inside an app's own templates.
//
// An app is free to add its own entries on top; it must not drop these
// three, or the shipped partials stop parsing.
func Funcs() template.FuncMap {
	return template.FuncMap{"dict": dict, "list": list, "icon": rastrillo.Icon}
}

// dict builds a map from alternating key/value arguments:
//
//	{{template "status-pill" dict "Tone" "positive" "Label" "Published"}}
//
// An odd argument count or a non-string key is an error, which
// html/template surfaces from Execute. Failing loudly beats silently
// dropping the trailing key — a partial rendering one field short is a
// bug that looks like a design decision.
func dict(pairs ...any) (map[string]any, error) {
	if len(pairs)%2 != 0 {
		return nil, fmt.Errorf("ui: dict wants an even number of arguments (key, value, …), got %d", len(pairs))
	}
	m := make(map[string]any, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok {
			return nil, fmt.Errorf("ui: dict key %d is %T, want string", i/2, pairs[i])
		}
		m[key] = pairs[i+1]
	}
	return m, nil
}

// list builds a slice from its arguments — pagination's Items, mostly:
//
//	{{template "pagination" dict "Items" (list
//	    (dict "Label" "1" "Current" true)
//	    (dict "Label" "2" "Href" "/posts?page=2"))}}
//
// It returns an empty, non-nil slice for no arguments so a template can
// range over the result unconditionally.
func list(items ...any) []any {
	out := make([]any, 0, len(items))
	return append(out, items...)
}
