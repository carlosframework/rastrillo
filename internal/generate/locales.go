package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/carlosframework/rastrillo/internal/catalog"
)

// MissingKeys reports, per non-default locale, the keys present in the
// default locale's catalog and absent from that one — design doc §10's
// pre-ship check: "silent fallback while iterating, loud failure before
// ship." The other direction (a key a non-default locale has and the
// default does not) is deliberately not a failure; §10 names only this
// one.
//
// Only the app's own catalogs are compared. The framework's base
// component catalog is never an app's to translate, and it is not in
// localesDir, so it cannot be reported here by construction.
//
// An app with no locales/ directory at all is the common single-locale
// case and returns no findings.
func MissingKeys(localesDir, defaultCode string) (map[string][]string, error) {
	entries, err := os.ReadDir(localesDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	catalogs := map[string]map[string]string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".toml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(localesDir, name))
		if err != nil {
			return nil, err
		}
		m, err := catalog.Decode(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		catalogs[strings.TrimSuffix(name, ".toml")] = m
	}
	if len(catalogs) == 0 {
		return nil, nil
	}

	def, ok := catalogs[defaultCode]
	if !ok {
		return nil, fmt.Errorf("no %s.toml in %s, but %d other locale catalog(s) are there — the default locale's catalog is what every other one is checked against", defaultCode, localesDir, len(catalogs))
	}

	out := map[string][]string{}
	for code, c := range catalogs {
		if code == defaultCode {
			continue
		}
		var missing []string
		for key := range def {
			if _, ok := c[key]; !ok {
				missing = append(missing, key)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			out[code] = missing
		}
	}
	return out, nil
}
