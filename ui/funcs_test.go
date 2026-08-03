package ui

import (
	"html/template"
	"strings"
	"testing"
)

func TestDictBuildsAMap(t *testing.T) {
	m, err := dict("Title", "Posts", "Count", 3)
	if err != nil {
		t.Fatalf("dict: %v", err)
	}
	if m["Title"] != "Posts" {
		t.Errorf(`m["Title"] = %v, want "Posts"`, m["Title"])
	}
	if m["Count"] != 3 {
		t.Errorf(`m["Count"] = %v, want 3`, m["Count"])
	}
	if len(m) != 2 {
		t.Errorf("len(m) = %d, want 2", len(m))
	}
}

func TestDictWithNoPairsIsEmptyNotNil(t *testing.T) {
	m, err := dict()
	if err != nil {
		t.Fatalf("dict: %v", err)
	}
	if m == nil {
		t.Fatal("dict() returned a nil map; callers index into it")
	}
	if len(m) != 0 {
		t.Errorf("len(m) = %d, want 0", len(m))
	}
}

// An odd argument count must fail loudly at Execute rather than silently
// dropping the last key (spec §4.1).
func TestDictOddArgCountIsAnError(t *testing.T) {
	if _, err := dict("Title", "Posts", "Count"); err == nil {
		t.Fatal("dict with 3 args returned no error")
	}
}

func TestDictNonStringKeyIsAnError(t *testing.T) {
	if _, err := dict(7, "Posts"); err == nil {
		t.Fatal("dict with a non-string key returned no error")
	}
}

func TestListBuildsASlice(t *testing.T) {
	got := list("a", 2, true)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0] != "a" || got[1] != 2 || got[2] != true {
		t.Errorf("list(...) = %v, want [a 2 true]", got)
	}
}

func TestListWithNoItemsIsEmptyNotNil(t *testing.T) {
	if got := list(); got == nil {
		t.Fatal("list() returned nil; templates range over this")
	}
}

func TestFuncsRegistersDictListAndIcon(t *testing.T) {
	f := Funcs()
	for _, name := range []string{"dict", "list", "icon"} {
		if _, ok := f[name]; !ok {
			t.Errorf("Funcs() is missing %q", name)
		}
	}
	if len(f) != 3 {
		t.Errorf("Funcs() has %d entries, want exactly 3", len(f))
	}
}

// The value proposition: these have to work as real FuncMap entries end
// to end, the same standard icons_test.go's TestIconWorksAsTemplateFunc
// holds Icon to.
func TestFuncsWorkEndToEnd(t *testing.T) {
	tmpl := template.Must(template.New("t").Funcs(Funcs()).Parse(
		`{{$d := dict "Label" "Search"}}{{$d.Label}}|{{len (list 1 2 3)}}|{{icon "search"}}`,
	))
	var buf strings.Builder
	if err := tmpl.Execute(&buf, nil); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := buf.String()
	if !strings.HasPrefix(got, "Search|3|") {
		t.Errorf("dict/list did not render as expected: %s", got)
	}
	if !strings.Contains(got, "<svg") {
		t.Errorf("icon did not resolve through Funcs(): %s", got)
	}
}

// An odd dict call must surface as an Execute error, not a silent render.
func TestDictErrorSurfacesAtExecute(t *testing.T) {
	tmpl := template.Must(template.New("t").Funcs(Funcs()).Parse(`{{$d := dict "A"}}{{$d.A}}`))
	var buf strings.Builder
	if err := tmpl.Execute(&buf, nil); err == nil {
		t.Fatal("Execute returned no error for an odd-argument dict call")
	}
}
