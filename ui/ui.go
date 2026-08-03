// Package ui is rastrillo's server-shape component library: a small
// starter set of List-screen html/template partials, a design-token
// stylesheet, and the template helpers they need — vendored the same way
// icons.go vendors Lucide, so an app pulls in a working component with an
// import and a ParseFS call, not a hand-copy.
//
// It is a component library, not a screen generator. Nothing here
// generates a screen, decides a route, or owns rendering. An app builds
// its own template tree and calls these partials by name:
//
//	tmpl := template.Must(template.New("").Funcs(ui.Funcs()).
//	        ParseFS(ui.Templates(), "*.html"))
//	tmpl = template.Must(tmpl.ParseFS(appTemplateFS, "templates/*.html"))
//
// The eight partials are page-header, list-bar, list-bar-search,
// list-search-submit, list-row-action, status-pill, empty-state and
// pagination. Each takes exactly one data value; build it inline with
// dict (see Funcs). Each partial's own file carries its data contract in
// a template comment above the {{define}}.
//
// Two container classes the partials assume but do not emit, because
// they belong to the app's own page markup:
//
//	<div class="rst-page">   — the centred content column every screen sits in
//	<div class="rst-list">   — the card wrapping a list-bar and a run of rows
//
// Styling comes from tokens.css, which rastrillo new writes once into a
// new app's static/ directory. rastrillo.Serve never serves it: from the
// moment it is scaffolded it is an ordinary app-owned static file the app
// is free to edit in place.
//
// Errors follow ordinary html/template semantics (nothing is
// special-cased). With dict-built map data a key the caller forgot to set
// does not fail at Execute; the partials guard every optional field, so a
// missing key renders nothing. A caller who wants missing-field detection
// gets it by passing a Go struct instead of a dict-built map.
package ui

import (
	"embed"
	"io/fs"
)

//go:embed partials/*.html
var partialsFS embed.FS

//go:embed tokens.css
var tokensCSS []byte

// Templates returns the embedded partials rooted at partials/, so every
// caller parses "*.html" regardless of this package's own source-tree
// layout:
//
//	tmpl := template.Must(template.New("").Funcs(ui.Funcs()).
//	        ParseFS(ui.Templates(), "*.html"))
func Templates() fs.FS {
	sub, err := fs.Sub(partialsFS, "partials")
	if err != nil {
		panic(err) // partials/ is embedded above; this cannot fail
	}
	return sub
}

// TokensCSS returns tokens.css's raw bytes, for rastrillo new's scaffold
// step to write into a new app's static directory. The stylesheet is
// delivered once, at scaffold time, and is app-owned from then on.
func TokensCSS() []byte { return tokensCSS }
