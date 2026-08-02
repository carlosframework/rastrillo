package rastrillo

import (
	"html/template"
	"strings"
	"testing"
)

// Every vendored icon is one self-contained, colour-inheriting,
// screen-reader-silent 24x24 SVG. Mirrors
// carlosframework/platform's internal/console/icons_test.go
// TestVendoredIconsAreSelfContained.
func TestVendoredIconsAreSelfContained(t *testing.T) {
	if len(icons) == 0 {
		t.Fatal("the icon set is empty")
	}
	for slug, svg := range icons {
		s := string(svg)
		for _, want := range []string{
			"<svg", "</svg>", `viewBox="0 0 24 24"`,
			`stroke="currentColor"`, `aria-hidden="true"`, `class="icon"`,
		} {
			if !strings.Contains(s, want) {
				t.Errorf("icon %q is missing %s: %s", slug, want, s)
			}
		}
		for _, bad := range []string{"http://", "https://", "<image", "xlink:href", "url("} {
			if strings.Contains(s, bad) {
				t.Errorf("icon %q reaches outside the page (%q): %s", slug, bad, s)
			}
		}
		if strings.Contains(s, " width=") || strings.Contains(s, " height=") {
			t.Errorf("icon %q hardcodes a size instead of sizing from the caller's CSS: %s", slug, s)
		}
	}
}

// An unknown slug must not panic a caller mid-render -- it renders nothing,
// same rule as console's icon().
func TestIconUnknownSlugRendersNothing(t *testing.T) {
	if got := Icon("not-a-real-icon"); got != "" {
		t.Errorf("Icon(%q) = %q, want empty string", "not-a-real-icon", got)
	}
}

// The actual value proposition: Icon must work as a real html/template
// FuncMap entry, end to end -- not just type-check as template.HTML.
func TestIconWorksAsTemplateFunc(t *testing.T) {
	tmpl := template.Must(
		template.New("t").Funcs(template.FuncMap{"icon": Icon}).Parse(`{{icon "check"}}`),
	)
	var buf strings.Builder
	if err := tmpl.Execute(&buf, nil); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "<svg") || !strings.Contains(got, `aria-hidden="true"`) {
		t.Errorf("rendered template output missing expected icon markup: %s", got)
	}
}
