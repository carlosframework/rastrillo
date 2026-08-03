package catalog

import (
	"reflect"
	"strings"
	"testing"
)

func TestDecode(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want map[string]string
	}{
		{
			name: "basic pairs",
			in:   "ui.save = \"Save\"\nui.cancel = \"Cancel\"\n",
			want: map[string]string{"ui.save": "Save", "ui.cancel": "Cancel"},
		},
		{
			name: "blank lines and comments",
			in:   "# a catalog\n\nui.save = \"Save\"\n\n# trailing note\n",
			want: map[string]string{"ui.save": "Save"},
		},
		{
			name: "dots in a key are literal, not table nesting",
			in:   "resource.ticket_types.field.price = \"Price\"\n",
			want: map[string]string{"resource.ticket_types.field.price": "Price"},
		},
		{
			name: "escapes in a basic string",
			in:   "k = \"one\\ntwo\\t\\\"quoted\\\"\"\n",
			want: map[string]string{"k": "one\ntwo\t\"quoted\""},
		},
		{
			name: "literal string keeps backslashes",
			in:   "k = 'C:\\path'\n",
			want: map[string]string{"k": `C:\path`},
		},
		{
			name: "trailing comment after a value",
			in:   "k = \"Save\"   # keep short\n",
			want: map[string]string{"k": "Save"},
		},
		{
			name: "hash inside a string is content, not a comment",
			in:   "k = \"tag #1\"\n",
			want: map[string]string{"k": "tag #1"},
		},
		{
			name: "CRLF line endings",
			in:   "k = \"Save\"\r\nj = \"Cancel\"\r\n",
			want: map[string]string{"k": "Save", "j": "Cancel"},
		},
		{
			name: "placeholder braces survive verbatim",
			in:   "ui.pagination.count = \"Page {page} of {pages}\"\n",
			want: map[string]string{"ui.pagination.count": "Page {page} of {pages}"},
		},
		{
			name: "empty input",
			in:   "",
			want: map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Decode([]byte(tt.in))
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Decode = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestDecodeErrors(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantSub string
	}{
		{"table header", "[list]\nk = \"v\"\n", "tables are not part"},
		{"unquoted value", "k = v\n", "must be a quoted string"},
		{"no equals", "just a line\n", `expected key = "value"`},
		{"duplicate key", "k = \"a\"\nk = \"b\"\n", `duplicate key "k"`},
		{"unterminated basic string", "k = \"oops\n", "unterminated string"},
		{"unterminated literal string", "k = 'oops\n", "unterminated literal string"},
		{"junk after value", "k = \"a\" b\n", "unexpected trailing content"},
		{"invalid key", "k y = \"a\"\n", "invalid key"},
		{"empty key", " = \"a\"\n", "invalid key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Decode([]byte(tt.in))
			if err == nil {
				t.Fatalf("want an error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantSub)
			}
			if !strings.HasPrefix(err.Error(), "line ") {
				t.Errorf("error = %q, want it to start with a line number", err)
			}
		})
	}
}
