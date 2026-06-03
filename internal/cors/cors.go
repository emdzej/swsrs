// Package cors provides a small CORS middleware for the relay's HTTP
// surfaces (admin API + discovery). It deliberately reuses the same
// AllowedOriginPatterns the WebSocket data plane uses, so operators
// configure browser origins once via SWSRS_ALLOWED_ORIGINS.
//
// Matching semantics mirror coder/websocket's OriginPatterns: each pattern
// is a glob (path.Match) compared against the request's Origin **host**
// (lowercased, including port if present). Examples:
//
//	app.example.com               → exact host match
//	*.example.com                 → any single-subdomain on example.com
//	localhost:*                   → any port on localhost
//	*                             → any origin (use with care; combined
//	                                with credentials=true this still
//	                                echoes the caller's origin, which the
//	                                CORS spec allows)
//
// Empty AllowedOriginPatterns means CORS is disabled (no headers set,
// preflight requests fall through to the mux).
package cors

import (
	"net/http"
	"net/url"
	"path"
	"strings"
)

// Middleware wraps an http.Handler and adds CORS support for the
// configured origins.
type Middleware struct {
	AllowedOriginPatterns []string
}

// Wrap returns next wrapped with CORS handling. If AllowedOriginPatterns
// is empty the middleware is a passthrough.
func (m *Middleware) Wrap(next http.Handler) http.Handler {
	if len(m.AllowedOriginPatterns) == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		// Always vary on Origin so caches don't serve a wrong-origin
		// response (matters even when we end up NOT setting CORS headers,
		// because some intermediaries cache by URL only).
		w.Header().Add("Vary", "Origin")

		if origin != "" && m.allowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")

			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				// Preflight: short-circuit with allow-* headers.
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				if reqHdrs := r.Header.Get("Access-Control-Request-Headers"); reqHdrs != "" {
					w.Header().Set("Access-Control-Allow-Headers", reqHdrs)
				} else {
					w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				}
				w.Header().Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// allowed reports whether the given Origin header value matches any of
// the configured patterns. The match is performed against the host
// portion (lowercased, including port).
func (m *Middleware) allowed(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Host)
	for _, p := range m.AllowedOriginPatterns {
		ok, err := path.Match(strings.ToLower(p), host)
		if err == nil && ok {
			return true
		}
	}
	return false
}
