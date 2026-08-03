package rastrillo

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// captured records what the app mux's handler saw for a request that
// reached it: the path after any locale-prefix strip, the locale
// Middleware resolved (empty when no locale middleware ran), and a
// translation lookup — the three things buildHandler's assembly can
// change without a live socket.
type captured struct {
	path, locale, translated string
}

// helloMux is an app mux with one route, /hello, whose handler stashes
// what it saw into cap.
func helloMux(cap *captured) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		cap.path = r.URL.Path
		cap.locale = LocaleFrom(r)
		cap.translated = T(r, "greeting")
		fmt.Fprint(w, "hi")
	})
	return mux
}

// get runs a GET against h and returns the recorder, so tests can assert
// on both the app-visible capture and the framework's own response body.
func get(h http.Handler, path, acceptLanguage string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	if acceptLanguage != "" {
		req.Header.Set("Accept-Language", acceptLanguage)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// frCatalog is the MapFS the brief specifies: a French catalog with one
// key, no English catalog — so English-locale lookups exercise the
// key-verbatim fallback while French exercises a real catalog hit.
var frCatalog = fstest.MapFS{
	"locales/fr.toml": &fstest.MapFile{Data: []byte("greeting = \"Bonjour\"\n")},
}

func TestBuildHandlerLocaleAssembly(t *testing.T) {
	tests := []struct {
		name           string
		locales        []string
		defaultLocale  string
		fsys           fstest.MapFS
		path           string
		acceptLanguage string
		wantPath       string
		wantLocale     string
		wantTranslated string
	}{
		{
			name:           "no locales: app sees the request untouched",
			path:           "/hello",
			wantPath:       "/hello",
			wantLocale:     "",
			wantTranslated: "greeting",
		},
		{
			name:           "prefix strip and catalog hit",
			locales:        []string{"en", "fr"},
			defaultLocale:  "en",
			fsys:           frCatalog,
			path:           "/fr/hello",
			wantPath:       "/hello",
			wantLocale:     "fr",
			wantTranslated: "Bonjour",
		},
		{
			name:           "Accept-Language negotiation, no prefix",
			locales:        []string{"en", "fr"},
			defaultLocale:  "en",
			fsys:           frCatalog,
			path:           "/hello",
			acceptLanguage: "fr-CA, en;q=0.5",
			wantPath:       "/hello",
			wantLocale:     "fr",
			wantTranslated: "Bonjour",
		},
		{
			name:           "default locale fallback, no catalog for it",
			locales:        []string{"en", "fr"},
			defaultLocale:  "en",
			fsys:           frCatalog,
			path:           "/hello",
			wantPath:       "/hello",
			wantLocale:     "en",
			wantTranslated: "greeting",
		},
		{
			name:           "empty DefaultLocale defaults to Locales[0]",
			locales:        []string{"fr"},
			fsys:           frCatalog,
			path:           "/hello",
			wantPath:       "/hello",
			wantLocale:     "fr",
			wantTranslated: "Bonjour",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cap captured
			opts := Options{
				Mux:           helloMux(&cap),
				Locales:       tt.locales,
				DefaultLocale: tt.defaultLocale,
				LocaleFS:      tt.fsys,
			}
			h, err := buildHandler(opts)
			if err != nil {
				t.Fatalf("buildHandler: %v", err)
			}
			get(h, tt.path, tt.acceptLanguage)
			if cap.path != tt.wantPath {
				t.Errorf("path = %q, want %q", cap.path, tt.wantPath)
			}
			if cap.locale != tt.wantLocale {
				t.Errorf("locale = %q, want %q", cap.locale, tt.wantLocale)
			}
			if cap.translated != tt.wantTranslated {
				t.Errorf("T = %q, want %q", cap.translated, tt.wantTranslated)
			}
		})
	}
}

// TestBuildHandlerHealthzUnaffectedByLocales covers the framework's own
// endpoints, both with no locale middleware installed and with one
// installed and a locale prefix present — /healthz must answer "ok"
// either way, and the prefix must strip before the framework mux sees
// the path, not just before the app mux does.
func TestBuildHandlerHealthzUnaffectedByLocales(t *testing.T) {
	tests := []struct {
		name    string
		locales []string
		path    string
	}{
		{"no locales", nil, "/healthz"},
		{"locales installed, no prefix", []string{"en", "fr"}, "/healthz"},
		{"locales installed, prefix strips first", []string{"en", "fr"}, "/fr/healthz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := Options{
				Mux:           helloMux(&captured{}),
				Locales:       tt.locales,
				DefaultLocale: "en",
			}
			h, err := buildHandler(opts)
			if err != nil {
				t.Fatalf("buildHandler: %v", err)
			}
			rec := get(h, tt.path, "")
			if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
				t.Errorf("GET %s = %d %q, want 200 \"ok\"", tt.path, rec.Code, rec.Body.String())
			}
		})
	}
}

// TestBuildHandlerNoLocalesNeverErrors documents the additive-only
// constraint from the other direction: Options.Locales unset must
// never reach NewLocales, so a nil LocaleFS or an empty DefaultLocale
// can never surface an error when no locale codes are declared.
func TestBuildHandlerNoLocalesNeverErrors(t *testing.T) {
	h, err := buildHandler(Options{Mux: helloMux(&captured{})})
	if err != nil {
		t.Fatalf("buildHandler with no Locales: %v", err)
	}
	if h == nil {
		t.Fatal("buildHandler returned a nil handler with no error")
	}
}

// TestBuildHandlerPropagatesLocaleErrors covers a malformed catalog file:
// buildHandler must surface NewLocales' error, and Serve's caller must
// be able to trust that message is not double-prefixed. NewLocales
// already wraps its errors as "rastrillo: ...", so buildHandler must
// return it as-is rather than wrapping a second time.
func TestBuildHandlerPropagatesLocaleErrors(t *testing.T) {
	badFS := fstest.MapFS{
		"locales/en.toml": &fstest.MapFile{Data: []byte("bad line\n")},
	}
	_, err := buildHandler(Options{
		Mux:      helloMux(&captured{}),
		Locales:  []string{"en"},
		LocaleFS: badFS,
	})
	if err == nil {
		t.Fatal("expected an error for a malformed catalog file")
	}
	if !strings.Contains(err.Error(), "locales/en.toml") {
		t.Errorf("error %q does not mention the offending file", err.Error())
	}
	if n := strings.Count(err.Error(), "rastrillo:"); n != 1 {
		t.Errorf("error %q has %d \"rastrillo:\" prefixes, want exactly 1", err.Error(), n)
	}
}
