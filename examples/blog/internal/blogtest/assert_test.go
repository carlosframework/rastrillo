package blogtest

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// The assertions every test in this package shares. Plain stdlib, no
// assertion library — the same style as the rastrillo repo's own tests.

func wantStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d", rec.Code, want)
	}
}

func wantContains(t *testing.T, body, want string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Errorf("missing %q in:\n%s", want, body)
	}
}

func wantNotContains(t *testing.T, body, unwanted string) {
	t.Helper()
	if strings.Contains(body, unwanted) {
		t.Errorf("unexpected %q in:\n%s", unwanted, body)
	}
}
