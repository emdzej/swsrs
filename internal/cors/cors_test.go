package cors_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/emdzej/swsrs/internal/cors"
)

func handlerOK() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

func TestPassthroughWhenNoOrigins(t *testing.T) {
	m := &cors.Middleware{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "https://app.example.com")
	m.Wrap(handlerOK()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected no ACAO header, got %q", got)
	}
	if got := rec.Header().Get("Vary"); got != "" {
		t.Errorf("expected no Vary header when disabled, got %q", got)
	}
}

func TestAllowedOriginGetsHeaders(t *testing.T) {
	m := &cors.Middleware{AllowedOriginPatterns: []string{"app.example.com"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "https://app.example.com")
	m.Wrap(handlerOK()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("ACAO = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("ACAC = %q", got)
	}
	if !strings.Contains(rec.Header().Get("Vary"), "Origin") {
		t.Errorf("Vary header missing Origin: %q", rec.Header().Get("Vary"))
	}
}

func TestDisallowedOriginNoHeaders(t *testing.T) {
	m := &cors.Middleware{AllowedOriginPatterns: []string{"app.example.com"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "https://evil.example.org")
	m.Wrap(handlerOK()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected no ACAO for disallowed origin, got %q", got)
	}
	// Vary: Origin should still be set (so caches don't poison)
	if !strings.Contains(rec.Header().Get("Vary"), "Origin") {
		t.Errorf("Vary header should include Origin even on disallowed: %q", rec.Header().Get("Vary"))
	}
}

func TestPreflightShortCircuit(t *testing.T) {
	m := &cors.Middleware{AllowedOriginPatterns: []string{"app.example.com"}}
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/admin/sessions", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Authorization, Content-Type")
	m.Wrap(next).ServeHTTP(rec, req)

	if called {
		t.Error("preflight should short-circuit, but next handler ran")
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("preflight status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, "POST") {
		t.Errorf("ACA-Methods = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Authorization, Content-Type" {
		t.Errorf("ACA-Headers = %q", got)
	}
}

func TestPreflightDisallowedFallsThrough(t *testing.T) {
	m := &cors.Middleware{AllowedOriginPatterns: []string{"app.example.com"}}
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/admin/sessions", nil)
	req.Header.Set("Origin", "https://evil.example.org")
	req.Header.Set("Access-Control-Request-Method", "POST")
	m.Wrap(next).ServeHTTP(rec, req)

	if !called {
		t.Error("disallowed origin should fall through, but next was not called")
	}
}

func TestNoOriginHeaderFallsThrough(t *testing.T) {
	m := &cors.Middleware{AllowedOriginPatterns: []string{"app.example.com"}}
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil) // no Origin header
	m.Wrap(next).ServeHTTP(rec, req)

	if !called {
		t.Error("no-Origin requests should pass through")
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("should not set ACAO when no Origin: %q", got)
	}
}

func TestWildcardPatternHostMatch(t *testing.T) {
	cases := []struct {
		pattern, origin string
		want            bool
	}{
		{"*.example.com", "https://app.example.com", true},
		{"*.example.com", "https://api.example.com", true},
		{"*.example.com", "https://example.com", false}, // glob `*` matches at least one char
		{"*.example.com", "https://app.evil.com", false},
		{"localhost:*", "http://localhost:3000", true},
		{"localhost:*", "http://localhost:8080", true},
		{"localhost:*", "http://127.0.0.1:3000", false},
	}
	for _, tc := range cases {
		m := &cors.Middleware{AllowedOriginPatterns: []string{tc.pattern}}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set("Origin", tc.origin)
		m.Wrap(handlerOK()).ServeHTTP(rec, req)
		got := rec.Header().Get("Access-Control-Allow-Origin") != ""
		if got != tc.want {
			t.Errorf("pattern=%q origin=%q: allowed=%v, want=%v", tc.pattern, tc.origin, got, tc.want)
		}
	}
}

func TestNonPreflightOptionsPassesThrough(t *testing.T) {
	// An OPTIONS request WITHOUT Access-Control-Request-Method shouldn't be
	// treated as a preflight; let the underlying handler decide.
	m := &cors.Middleware{AllowedOriginPatterns: []string{"app.example.com"}}
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/x", nil)
	req.Header.Set("Origin", "https://app.example.com")
	m.Wrap(next).ServeHTTP(rec, req)
	if !called {
		t.Error("plain OPTIONS (not preflight) should pass through to handler")
	}
}
