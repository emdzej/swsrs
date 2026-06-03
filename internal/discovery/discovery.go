// Package discovery serves /.well-known/swsrs-config — a small JSON
// document that lets clients drive OAuth flows themselves (device flow,
// PKCE) without being hard-coded against a specific IdP.
//
// The document is public; it carries no secrets. It does carry the IdP
// endpoint URLs (already public anyway, since the issuer is configured at
// deploy time) and a shared client_id_hint when configured.
package discovery

import (
	"encoding/json"
	"net/http"

	"github.com/emdzej/swsrs/internal/auth"
)

// Config is the JSON shape of /.well-known/swsrs-config.
type Config struct {
	Issuer                      string   `json:"issuer,omitempty"`
	Audience                    string   `json:"audience,omitempty"`
	Scopes                      []string `json:"scopes,omitempty"`
	AuthorizationEndpoint       string   `json:"authorization_endpoint,omitempty"`
	TokenEndpoint               string   `json:"token_endpoint,omitempty"`
	DeviceAuthorizationEndpoint string   `json:"device_authorization_endpoint,omitempty"`
	// ClientIDHint is a shared OAuth client_id that all swsrs clients of
	// this deployment may use against the IdP. Optional. When empty,
	// callers must bring their own client_id.
	ClientIDHint string `json:"client_id_hint,omitempty"`
}

// Handler returns an HTTP handler that emits the discovery document.
// scopes lists the admin-API scopes any client will need.
// clientIDHint is optional; empty values are omitted from the JSON.
//
// If verifier is nil (server running --no-auth), the handler returns 404 —
// there's nothing to discover.
func Handler(verifier *auth.Verifier, scopes []string, clientIDHint string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if verifier == nil {
			http.Error(w, "auth disabled on this deployment (--no-auth)", http.StatusNotFound)
			return
		}
		ep := verifier.Endpoints()
		cfg := Config{
			Issuer:                      verifier.Issuer(),
			Audience:                    verifier.Audience(),
			Scopes:                      scopes,
			AuthorizationEndpoint:       ep.AuthorizationEndpoint,
			TokenEndpoint:               ep.TokenEndpoint,
			DeviceAuthorizationEndpoint: ep.DeviceAuthorizationEndpoint,
			ClientIDHint:                clientIDHint,
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=300")
		_ = json.NewEncoder(w).Encode(cfg)
	})
}
