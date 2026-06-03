// Package auth provides OIDC JWT verification for the admin API.
//
// Session-connect tokens are NOT verified here — those are opaque per-slot
// tokens checked directly by the session store. See [[auth-split]].
package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
)

type Verifier struct {
	verifier *oidc.IDTokenVerifier
	audience string
	issuer   string
	// IdP endpoints captured from the discovery document for surfacing
	// via /.well-known/swsrs-config. Empty if the IdP doesn't advertise them.
	endpoints IdPEndpoints
}

// IdPEndpoints holds the subset of OIDC discovery fields that clients need
// to drive OAuth flows themselves.
type IdPEndpoints struct {
	AuthorizationEndpoint       string `json:"authorization_endpoint,omitempty"`
	TokenEndpoint               string `json:"token_endpoint,omitempty"`
	DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint,omitempty"`
}

// NewVerifier autodiscovers OIDC config from issuer URL and returns a
// verifier configured for the given audience (client_id).
func NewVerifier(ctx context.Context, issuer, audience string) (*Verifier, error) {
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}
	cfg := &oidc.Config{
		ClientID: audience,
		// SkipClientIDCheck if audience is empty — useful for dev/test only.
		SkipClientIDCheck: audience == "",
	}

	// Capture endpoints we want to expose to clients. The provider's
	// public Endpoint() gives auth + token; device_authorization_endpoint
	// only lives in the raw discovery doc.
	ep := provider.Endpoint()
	var raw struct {
		DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint"`
	}
	_ = provider.Claims(&raw)

	return &Verifier{
		verifier: provider.Verifier(cfg),
		audience: audience,
		issuer:   issuer,
		endpoints: IdPEndpoints{
			AuthorizationEndpoint:       ep.AuthURL,
			TokenEndpoint:               ep.TokenURL,
			DeviceAuthorizationEndpoint: raw.DeviceAuthorizationEndpoint,
		},
	}, nil
}

// Issuer returns the configured issuer URL.
func (v *Verifier) Issuer() string { return v.issuer }

// Audience returns the configured audience (empty if disabled).
func (v *Verifier) Audience() string { return v.audience }

// Endpoints returns the IdP endpoints captured at discovery time.
func (v *Verifier) Endpoints() IdPEndpoints { return v.endpoints }

// Claims holds the fields we care about from a verified token.
type Claims struct {
	Subject string   `json:"sub"`
	Scopes  []string `json:"-"`
	Raw     map[string]any
}

// Verify validates the bearer token and returns parsed claims.
func (v *Verifier) Verify(ctx context.Context, bearer string) (*Claims, error) {
	tok, err := v.verifier.Verify(ctx, bearer)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := tok.Claims(&raw); err != nil {
		return nil, fmt.Errorf("decode claims: %w", err)
	}
	c := &Claims{Subject: tok.Subject, Raw: raw}
	c.Scopes = extractScopes(raw)
	return c, nil
}

// extractScopes pulls scopes from either the `scope` (space-delimited string,
// per RFC 8693) or `scp` (array, Azure-style) claim.
func extractScopes(raw map[string]any) []string {
	if v, ok := raw["scope"].(string); ok {
		return strings.Fields(v)
	}
	if arr, ok := raw["scp"].([]any); ok {
		out := make([]string, 0, len(arr))
		for _, s := range arr {
			if str, ok := s.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}

// HasScope reports whether the token includes the given scope.
func (c *Claims) HasScope(s string) bool {
	for _, x := range c.Scopes {
		if x == s {
			return true
		}
	}
	return false
}

// Middleware returns an http handler that verifies the Authorization bearer
// token and ensures the listed scope is present. The verified Claims are
// attached to the request context.
func (v *Verifier) Middleware(scope string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bearer, err := bearerFromHeader(r.Header.Get("Authorization"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		claims, err := v.Verify(r.Context(), bearer)
		if err != nil {
			http.Error(w, "invalid token: "+err.Error(), http.StatusUnauthorized)
			return
		}
		if scope != "" && !claims.HasScope(scope) {
			http.Error(w, "missing required scope: "+scope, http.StatusForbidden)
			return
		}
		ctx := context.WithValue(r.Context(), claimsKey{}, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type claimsKey struct{}

func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(claimsKey{}).(*Claims)
	return c, ok
}

func bearerFromHeader(h string) (string, error) {
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return "", errors.New("missing bearer token")
	}
	return strings.TrimSpace(h[len(prefix):]), nil
}
