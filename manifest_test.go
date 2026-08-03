package rastrillo

import (
	"strings"
	"testing"
)

func validResource() Resource {
	return Resource{
		Name:  "notes",
		Route: "/admin/notes",
		Store: Exclusive,
		List: List{
			Columns: []Column{{Field: "Title"}, {Field: "Price", Kind: Money}},
			Search:  true,
			Filter:  []string{"Title"},
		},
		Form: Form{
			Basics:   []Field{{Name: "Title"}, {Name: "Body", Kind: Textarea}},
			Advanced: []Field{{Name: "Price", Kind: Money}},
		},
	}
}

func TestValidateAcceptsTheFixture(t *testing.T) {
	r := validResource()
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateRejections(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Resource)
		want string // substring of the error
	}{
		{"empty name", func(r *Resource) { r.Name = "" }, "name"},
		{"non-snake name", func(r *Resource) { r.Name = "TicketTypes" }, "snake_case"},
		{"empty route", func(r *Resource) { r.Route = "" }, "route"},
		{"trailing slash", func(r *Resource) { r.Route = "/admin/notes/" }, "trailing"},
		{"no leading slash", func(r *Resource) { r.Route = "admin/notes" }, "route"},
		{"mergeable", func(r *Resource) { r.Store = Mergeable }, "not yet built"},
		{"unknown store", func(r *Resource) { r.Store = "weird" }, "store"},
		{"unknown kind", func(r *Resource) { r.List.Columns[0].Kind = "meter" }, "kind"},
		{"filter not a column", func(r *Resource) { r.List.Filter = []string{"Status"} }, "filter"},
		{"nothing declared", func(r *Resource) { r.List = List{}; r.Form = Form{} }, "at least one"},
		{"duplicate field", func(r *Resource) { r.Form.Basics = append(r.Form.Basics, Field{Name: "Title"}) }, "duplicate"},
		{"non-identifier field", func(r *Resource) { r.Form.Basics[0].Name = "my-field" }, "identifier"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := validResource()
			tc.mut(&r)
			err := r.Validate()
			if err == nil {
				t.Fatal("Validate accepted it")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q missing %q", err, tc.want)
			}
		})
	}
}
