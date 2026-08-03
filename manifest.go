package rastrillo

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// RenderFunc is how a generated action hands a page to the app's own
// template tree — the seam generated code needs because it cannot
// call an app-private helper (a hand-rolled blog.Render, say).
// Ctx.Render carries it; the app's ctx factory sets it, and a
// generated action nil-checks it before use. page is always one of
// "<resource>/list", "<resource>/show" or "<resource>/form" —
// internal/generate's action emitter documents and pins the exact
// contract (see actions.go).
type RenderFunc func(ctx *Ctx, w http.ResponseWriter, page string, status int, data any)

var (
	namePattern    = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	segmentPattern = regexp.MustCompile(`^[a-z0-9_-]+$`)
	identPattern   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)
)

// Kind categorizes the input type for a column or form field.
type Kind string

const (
	Text     Kind = "text"
	Textarea Kind = "textarea"
	Money    Kind = "money"
)

// StoreKind categorizes how a resource's data is stored and synchronized.
type StoreKind string

const (
	Exclusive StoreKind = "exclusive"
	Mergeable StoreKind = "mergeable"
)

// Resource is one manifest: the §9 sugar a route opts into. Its JSON
// encoding (the struct tags here and on the types it embeds) is the
// generator's stable artifact — gen/manifest.json — consumed by any
// renderer; evolution is additive only. It describes a CRUD interface
// for a data entity.
type Resource struct {
	Name  string    `json:"name" toml:"name"`
	Route string    `json:"route" toml:"route"`
	Store StoreKind `json:"store" toml:"store"`
	List  List      `json:"list" toml:"list"`
	Form  Form      `json:"form" toml:"form"`
}

// List describes the table view for a resource.
type List struct {
	Columns []Column `json:"columns" toml:"columns"`
	Search  bool     `json:"search" toml:"search"`
	Filter  []string `json:"filter" toml:"filter"`
}

// Column describes a column in a resource list.
type Column struct {
	Field string `json:"field" toml:"field"`
	Kind  Kind   `json:"kind" toml:"kind"` // zero value means Text
}

// Form describes the form views for creating and editing a resource.
type Form struct {
	Basics   []Field `json:"basics" toml:"basics"`
	Advanced []Field `json:"advanced" toml:"advanced"`
}

// Field describes an input field in a form.
type Field struct {
	Name string `json:"name" toml:"name"`
	Kind Kind   `json:"kind" toml:"kind"` // zero value means Text
}

// Validate checks the resource declaration for consistency and validity.
// It normalizes zero values for Kind and Store in place.
func (r *Resource) Validate() error {
	// Zero values normalize: empty Kind → Text, empty Store → Exclusive
	if r.Store == "" {
		r.Store = Exclusive
	}
	for i := range r.List.Columns {
		if r.List.Columns[i].Kind == "" {
			r.List.Columns[i].Kind = Text
		}
	}
	for i := range r.Form.Basics {
		if r.Form.Basics[i].Kind == "" {
			r.Form.Basics[i].Kind = Text
		}
	}
	for i := range r.Form.Advanced {
		if r.Form.Advanced[i].Kind == "" {
			r.Form.Advanced[i].Kind = Text
		}
	}

	if r.Name == "" {
		return fmt.Errorf("name: must not be empty")
	}
	if !namePattern.MatchString(r.Name) {
		return fmt.Errorf("name: must be snake_case")
	}

	if r.Route == "" {
		return fmt.Errorf("route: must not be empty")
	}
	if !strings.HasPrefix(r.Route, "/") {
		return fmt.Errorf("route: must start with /")
	}
	if strings.HasSuffix(r.Route, "/") && r.Route != "/" {
		return fmt.Errorf("route: must not have trailing slash")
	}
	// Route segments must be either {param} or [a-z0-9_-]+
	segments := strings.Split(strings.TrimPrefix(r.Route, "/"), "/")
	for _, seg := range segments {
		if seg == "" {
			continue
		}
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			continue
		}
		if !segmentPattern.MatchString(seg) {
			return fmt.Errorf("route: invalid segment %q", seg)
		}
	}

	if r.Store == Mergeable {
		return fmt.Errorf("store: mergeable is not yet built")
	}
	if r.Store != Exclusive && r.Store != Mergeable {
		return fmt.Errorf("store: unknown value %q", r.Store)
	}

	for _, col := range r.List.Columns {
		if col.Kind != Text && col.Kind != Textarea && col.Kind != Money {
			return fmt.Errorf("kind: unknown value %q", col.Kind)
		}
	}
	for _, fld := range r.Form.Basics {
		if fld.Kind != Text && fld.Kind != Textarea && fld.Kind != Money {
			return fmt.Errorf("kind: unknown value %q", fld.Kind)
		}
	}
	for _, fld := range r.Form.Advanced {
		if fld.Kind != Text && fld.Kind != Textarea && fld.Kind != Money {
			return fmt.Errorf("kind: unknown value %q", fld.Kind)
		}
	}

	if len(r.List.Columns) == 0 && len(r.Form.Basics) == 0 && len(r.Form.Advanced) == 0 {
		return fmt.Errorf("declaration: must have at least one list column or form field")
	}

	columnFields := make(map[string]bool)
	for _, col := range r.List.Columns {
		columnFields[col.Field] = true
	}

	for _, f := range r.List.Filter {
		if !columnFields[f] {
			return fmt.Errorf("filter: %q is not a declared column", f)
		}
	}

	// Field and column names must be valid identifiers
	for _, col := range r.List.Columns {
		if !identPattern.MatchString(col.Field) {
			return fmt.Errorf("name: %q is not a valid identifier", col.Field)
		}
	}
	for _, fld := range r.Form.Basics {
		if !identPattern.MatchString(fld.Name) {
			return fmt.Errorf("name: %q is not a valid identifier", fld.Name)
		}
	}
	for _, fld := range r.Form.Advanced {
		if !identPattern.MatchString(fld.Name) {
			return fmt.Errorf("name: %q is not a valid identifier", fld.Name)
		}
	}

	// Form fields (Basics + Advanced combined) must have no case-insensitive duplicates;
	// columns may repeat as form fields, but the form must not have duplicates.
	formFieldsLower := make(map[string]bool)
	for _, fld := range r.Form.Basics {
		lower := strings.ToLower(fld.Name)
		if formFieldsLower[lower] {
			return fmt.Errorf("name: duplicate %q (case-insensitive)", fld.Name)
		}
		formFieldsLower[lower] = true
	}
	for _, fld := range r.Form.Advanced {
		lower := strings.ToLower(fld.Name)
		if formFieldsLower[lower] {
			return fmt.Errorf("name: duplicate %q (case-insensitive)", fld.Name)
		}
		formFieldsLower[lower] = true
	}

	// Columns must have no case-insensitive duplicates among themselves
	columnFieldsLower := make(map[string]bool)
	for _, col := range r.List.Columns {
		lower := strings.ToLower(col.Field)
		if columnFieldsLower[lower] {
			return fmt.Errorf("name: duplicate %q (case-insensitive)", col.Field)
		}
		columnFieldsLower[lower] = true
	}

	return nil
}
