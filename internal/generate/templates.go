// Package generate's template emitter (this file) turns a validated
// rastrillo.Resource into the three screens a manifest owns —
// gen/templates/<name>/{list,show,form}.html — composed entirely from
// ui's partials (dict/list/icon come from ui.Funcs(), which the app
// registers alongside its own template tree; see ui/ui.go).
//
// Two kinds of string live in a generated template, and they are never
// mixed: strings the RESOURCE SHAPE decides (a screen's title, a field's
// label, a button's text) are known at generation time and baked in as
// literal `{{T "resource.<name>...."}}` / `{{T "ui...."}}` calls —
// EmitLocales (a later task) owns emitting the catalog keys these
// reference. Strings a RECORD decides (a row's Main/Sub, a field's
// current Value, a validation Error) are runtime data the action
// (a later task) computes and hands to Execute; the template only
// names the data key, it never re-derives or formats the value —
// Money in particular is a formatted dollar string by the time it
// reaches a template, never math the template does itself.
//
// list.html's data contract (pinned by the plan's golden, byte-exact):
// Empty bool; Query string; Carry [][2]string; Rows []struct{Href, Main,
// Sub string}; Pagination struct{Show bool; Items []struct{...}} (see
// ui/partials/pagination.html for Items' own shape). The v1 amendment
// (binding, recorded in the plan's self-review): list-bar renders
// Search only — no Filter key — because a manifest's `filter` entry
// validates but declares no enumerable values for a dropdown yet.
//
// show.html and form.html have no golden in the plan; this emitter
// pins their contracts (each file's own DO-NOT-EDIT comment restates
// it, since Task 8's action emitter is the contract's only reader
// besides this file):
//
//	show.html: Title string (the record's first text field's value —
//	list.html's Main, restated for the header); EditHref string (this
//	record's edit route); Fields map[string]string (every declared
//	field's current value, keyed by its declared name).
//
//	form.html: IsNew bool; Fields map[string]string (current values,
//	empty for New); Errors map[string]string, optional (a 400
//	re-render's per-field message); BasicsAction/AdvancedAction string,
//	meaningful only when !IsNew (Edit's two POST targets — Advanced's
//	only exists in the template at all when the resource declares
//	Advanced fields).
package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/carlosframework/rastrillo"
)

// EmitTemplates writes gen/templates/<name>/{list,show,form}.html for
// r. A hand-written templates/<name>/<file>.html already present under
// appRoot skips generating that one file — its computed gen/ path is
// reported in skipped (for --check), and nothing is written or touched
// there.
func EmitTemplates(appRoot, genDir string, r rastrillo.Resource) (written, skipped []string, err error) {
	genResDir := filepath.Join(genDir, "templates", r.Name)
	handResDir := filepath.Join(appRoot, "templates", r.Name)

	files := []struct {
		name    string
		content func(rastrillo.Resource) []byte
	}{
		{"list.html", listHTML},
		{"show.html", showHTML},
		{"form.html", formHTML},
	}

	for _, f := range files {
		genPath := filepath.Join(genResDir, f.name)

		_, statErr := os.Stat(filepath.Join(handResDir, f.name))
		if statErr == nil {
			skipped = append(skipped, genPath)
			continue
		}
		if !os.IsNotExist(statErr) {
			return nil, nil, fmt.Errorf("%s: %s: %w", r.Name, f.name, statErr)
		}

		if err := writeFileIfChanged(genPath, f.content(r)); err != nil {
			return nil, nil, fmt.Errorf("%s: %s: %w", r.Name, f.name, err)
		}
		written = append(written, genPath)
	}

	return written, skipped, nil
}

// resourceKey builds a `resource.<name>.<suffix>` catalog key — the
// only key shape generated templates ever reference for resource-owned
// text (shared chrome uses the `ui.*` keys instead, spelled literally
// at each call site since they don't vary by resource).
func resourceKey(name, suffix string) string {
	return "resource." + name + "." + suffix
}

// fieldPartial names the ui partial that renders k's field: Textarea
// gets the multi-line control, everything else (Text, Money — Money
// is display text the action already formatted, never a numeric
// input) gets field-text. Every Kind Validate accepts has an entry;
// an unrecognized Kind means Validate's accepted set and this emitter
// have drifted, so it panics rather than silently guessing — the same
// discipline store.go's kindSQLType uses for the same reason.
func fieldPartial(k rastrillo.Kind) string {
	switch k {
	case rastrillo.Text, rastrillo.Money:
		return "field-text"
	case rastrillo.Textarea:
		return "field-textarea"
	default:
		panic(fmt.Sprintf("generate: unknown Kind %q", k))
	}
}

// fieldMono reports whether k's value is Mono in a detail-list: Money
// is a formatted figure, not prose, so it renders monospace like any
// other machine-ish value. Same unrecognized-Kind panic as fieldPartial.
func fieldMono(k rastrillo.Kind) bool {
	switch k {
	case rastrillo.Text, rastrillo.Textarea:
		return false
	case rastrillo.Money:
		return true
	default:
		panic(fmt.Sprintf("generate: unknown Kind %q", k))
	}
}

// listHTML renders the List screen: page-header with the primary "new"
// action, an empty-state when there are no rows, otherwise list-bar
// (search only — see the package doc's v1 amendment) over a card of
// list-row-action rows and, when the action says so, pagination.
func listHTML(r rastrillo.Resource) []byte {
	newHref := r.Route + "/new"

	var b strings.Builder
	fmt.Fprintf(&b, "{{/* Code generated by rastrillo generate; DO NOT EDIT.\n")
	fmt.Fprintf(&b, "     Eject: copy to templates/%s/list.html and edit — generation of\n", r.Name)
	b.WriteString("     this one file then stops. */}}\n")
	b.WriteString("{{define \"content\"}}\n")
	fmt.Fprintf(&b, "{{template \"page-header\" dict \"Title\" (T %q) \"ActionHref\" %q \"ActionLabel\" (T \"ui.new\")}}\n",
		resourceKey(r.Name, "name"), newHref)
	b.WriteString("{{if .Empty}}\n")
	fmt.Fprintf(&b, "{{template \"empty-state\" dict \"Title\" (T %q) \"Body\" (T %q) \"ActionHref\" %q \"ActionLabel\" (T \"ui.new\")}}\n",
		resourceKey(r.Name, "empty.title"), resourceKey(r.Name, "empty.body"), newHref)
	b.WriteString("{{else}}\n")
	b.WriteString("<div class=\"rst-list\">\n")
	fmt.Fprintf(&b, "{{template \"list-bar\" dict \"SearchAction\" %q \"Query\" .Query \"Placeholder\" (T \"ui.search\") \"Hidden\" .Carry}}\n", r.Route)
	b.WriteString("{{range .Rows}}{{template \"list-row-action\" dict \"Href\" .Href \"Main\" .Main \"Sub\" .Sub}}\n")
	b.WriteString("{{end}}\n")
	b.WriteString("</div>\n")
	b.WriteString("{{if .Pagination.Show}}{{template \"pagination\" dict \"Items\" .Pagination.Items}}{{end}}\n")
	b.WriteString("{{end}}\n")
	b.WriteString("{{end}}\n")
	return []byte(b.String())
}

// showHTML renders the Show screen: page-header (the record's own
// title, an edit pill) over a detail-list of every declared field, in
// columns(r)'s order — the same union+order store.go's schema uses, so
// the screen shows exactly the columns the table has.
func showHTML(r rastrillo.Resource) []byte {
	cols := columns(r)

	names := make([]string, len(cols))
	var items strings.Builder
	for i, c := range cols {
		names[i] = c.Name
		if i > 0 {
			items.WriteString("\n")
		}
		extra := ""
		if fieldMono(c.Kind) {
			extra = " \"Mono\" true"
		}
		fmt.Fprintf(&items, "  (dict \"Label\" (T %q) \"Value\" .Fields.%s%s)",
			resourceKey(r.Name, "field."+c.SQL), c.Name, extra)
	}

	var b strings.Builder
	b.WriteString("{{/* Code generated by rastrillo generate; DO NOT EDIT.\n")
	fmt.Fprintf(&b, "     Eject: copy to templates/%s/show.html and edit — generation of\n", r.Name)
	b.WriteString("     this one file then stops.\n\n")
	b.WriteString("     Data contract (the action emitter reads this):\n")
	b.WriteString("       Title     string, required — the record shown here (its first\n")
	b.WriteString("                 text field's value; list.html's Main, restated for the\n")
	b.WriteString("                 header)\n")
	b.WriteString("       EditHref  string, required — this record's edit route\n")
	fmt.Fprintf(&b, "       Fields    map[string]string, required — every declared field's\n")
	fmt.Fprintf(&b, "                 value keyed by its declared name, in this resource's\n")
	fmt.Fprintf(&b, "                 declaration order: %s. Money is\n", strings.Join(names, ", "))
	b.WriteString("                 already formatted as a dollar string by the action —\n")
	b.WriteString("                 templates never do money math. */}}\n")
	b.WriteString("{{define \"content\"}}\n")
	b.WriteString("{{template \"page-header\" dict \"Title\" .Title \"ActionHref\" .EditHref \"ActionLabel\" (T \"ui.edit\")}}\n")
	b.WriteString("{{template \"detail-list\" dict \"Items\" (list\n")
	b.WriteString(items.String())
	b.WriteString("\n)}}\n")
	b.WriteString("{{end}}\n")
	return []byte(b.String())
}

// formField renders one field-text/field-textarea call for f, wired to
// its Fields/Errors data keys and its resource-key label.
func formField(r rastrillo.Resource, f rastrillo.Field) string {
	return fmt.Sprintf("{{template %q dict \"Name\" %q \"Label\" (T %q) \"Value\" .Fields.%s \"Error\" .Errors.%s}}\n",
		fieldPartial(f.Kind), f.Name, resourceKey(r.Name, "field."+sqlName(f.Name)), f.Name, f.Name)
}

// formFoot renders one form-foot call. Cancel always targets the list
// (r.Route) — the plan's contract for both New and Edit forms.
func formFoot(r rastrillo.Resource) string {
	return fmt.Sprintf("{{template \"form-foot\" dict \"Submit\" (T \"ui.save\") \"CancelHref\" %q \"CancelLabel\" (T \"ui.cancel\")}}\n", r.Route)
}

// formHTML renders the New/Edit screen. New is one form posting every
// field (Basics then Advanced) to the create route (r.Route, same path
// index.POST claims). Edit is always a Basics form posting to the
// data-supplied .BasicsAction, and — only when r.Form.Advanced is
// non-empty, decided here at generation time since that fact never
// changes at runtime — a second form posting Advanced fields to
// .AdvancedAction.
func formHTML(r rastrillo.Resource) []byte {
	hasAdvanced := len(r.Form.Advanced) > 0

	var b strings.Builder
	b.WriteString("{{/* Code generated by rastrillo generate; DO NOT EDIT.\n")
	fmt.Fprintf(&b, "     Eject: copy to templates/%s/form.html and edit — generation of\n", r.Name)
	b.WriteString("     this one file then stops.\n\n")
	b.WriteString("     Data contract (the action emitter reads this):\n")
	b.WriteString("       IsNew           bool, required — New renders one form posting\n")
	b.WriteString("                       every field to the create route; Edit renders\n")
	b.WriteString("                       Basics (and, when this resource declares\n")
	b.WriteString("                       Advanced fields, a second Advanced form)\n")
	b.WriteString("       Fields          map[string]string, required — current values\n")
	b.WriteString("                       keyed by declared field name; Money already\n")
	b.WriteString("                       formatted as a dollar string by the action\n")
	b.WriteString("                       (templates never do money math); empty strings\n")
	b.WriteString("                       for New\n")
	b.WriteString("       Errors          map[string]string, optional — a validation\n")
	b.WriteString("                       message per field name, set only on a 400\n")
	b.WriteString("                       re-render\n")
	if hasAdvanced {
		b.WriteString("       BasicsAction    string, required when !IsNew — the\n")
		b.WriteString("                       edit-basics POST target\n")
		b.WriteString("       AdvancedAction  string, required when !IsNew — the\n")
		b.WriteString("                       edit-advanced POST target */}}\n")
	} else {
		b.WriteString("       BasicsAction    string, required when !IsNew — the\n")
		b.WriteString("                       edit-basics POST target (this resource has\n")
		b.WriteString("                       no Advanced fields, so there is no second\n")
		b.WriteString("                       form) */}}\n")
	}
	b.WriteString("{{define \"content\"}}\n")
	b.WriteString("{{if .IsNew}}\n")
	fmt.Fprintf(&b, "<form class=\"rst-form\" method=\"post\" action=%q>\n", r.Route)
	for _, f := range r.Form.Basics {
		b.WriteString(formField(r, f))
	}
	for _, f := range r.Form.Advanced {
		b.WriteString(formField(r, f))
	}
	b.WriteString(formFoot(r))
	b.WriteString("</form>\n")
	b.WriteString("{{else}}\n")
	b.WriteString("<form class=\"rst-form\" method=\"post\" action=\"{{.BasicsAction}}\">\n")
	for _, f := range r.Form.Basics {
		b.WriteString(formField(r, f))
	}
	b.WriteString(formFoot(r))
	b.WriteString("</form>\n")
	if hasAdvanced {
		b.WriteString("<form class=\"rst-form\" method=\"post\" action=\"{{.AdvancedAction}}\">\n")
		for _, f := range r.Form.Advanced {
			b.WriteString(formField(r, f))
		}
		b.WriteString(formFoot(r))
		b.WriteString("</form>\n")
	}
	b.WriteString("{{end}}\n")
	b.WriteString("{{end}}\n")
	return []byte(b.String())
}
