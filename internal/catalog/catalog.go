// Package catalog decodes the flat `key = "string"` TOML subset
// rastrillo's locale catalogs use (design doc §10: "One TOML file per
// locale ... flat key → string"; §14: "No pluralization/ICU-style
// grammar rules in the localization catalog — flat key → string only,
// v1").
//
// This is deliberately NOT a general TOML decoder, and must not grow
// into one. The design doc's open question 1 (§15) — whether the
// *manifest* system takes a TOML dependency or hand-rolls a decoder —
// is about a format with tables, arrays and inline tables, and stays
// open. Catalogs need none of that, so a page of code covers them and
// deferring buys nothing.
package catalog

import (
	"fmt"
	"strconv"
	"strings"
)

// Decode parses one catalog file: one `key = "value"` per line, plus
// blank lines and # comments. A dot inside a key is a literal character,
// not table nesting — `resource.ticket_types.field.price` is one flat
// key, which is exactly the key shape design doc §10 specifies.
func Decode(data []byte) (map[string]string, error) {
	out := map[string]string{}
	for i, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			return nil, fmt.Errorf("line %d: tables are not part of the flat catalog format", i+1)
		}
		eq := strings.Index(line, "=")
		if eq < 0 {
			return nil, fmt.Errorf(`line %d: expected key = "value"`, i+1)
		}
		key := strings.TrimSpace(line[:eq])
		if !validKey(key) {
			return nil, fmt.Errorf("line %d: invalid key %q", i+1, key)
		}
		val, err := parseValue(strings.TrimSpace(line[eq+1:]))
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		if _, dup := out[key]; dup {
			return nil, fmt.Errorf("line %d: duplicate key %q", i+1, key)
		}
		out[key] = val
	}
	return out, nil
}

// validKey accepts TOML bare-key characters plus the dot, which catalogs
// use as a flat namespace separator rather than as nesting.
func validKey(k string) bool {
	if k == "" {
		return false
	}
	for _, r := range k {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_', r == '-', r == '.':
		default:
			return false
		}
	}
	return true
}

// parseValue accepts a TOML basic string ("..." with backslash escapes)
// or a literal string ('...', no escapes), optionally followed by a
// # comment. The closing quote is found before any comment is
// considered, so a # inside a translated string stays content.
func parseValue(s string) (string, error) {
	switch {
	case strings.HasPrefix(s, `"`):
		end := -1
		for i := 1; i < len(s); i++ {
			if s[i] == '\\' {
				i++
				continue
			}
			if s[i] == '"' {
				end = i
				break
			}
		}
		if end < 0 {
			return "", fmt.Errorf("unterminated string")
		}
		if err := checkTrailer(s[end+1:]); err != nil {
			return "", err
		}
		// TOML basic-string escapes are a subset of Go's interpreted
		// string literal escapes, so Unquote is exactly right here.
		return strconv.Unquote(s[:end+1])
	case strings.HasPrefix(s, "'"):
		end := strings.Index(s[1:], "'")
		if end < 0 {
			return "", fmt.Errorf("unterminated literal string")
		}
		if err := checkTrailer(s[end+2:]); err != nil {
			return "", err
		}
		return s[1 : end+1], nil
	default:
		return "", fmt.Errorf("value must be a quoted string")
	}
}

func checkTrailer(rest string) error {
	rest = strings.TrimSpace(rest)
	if rest == "" || strings.HasPrefix(rest, "#") {
		return nil
	}
	return fmt.Errorf("unexpected trailing content %q", rest)
}
