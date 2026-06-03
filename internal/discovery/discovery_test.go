package discovery_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/emdzej/swsrs/internal/auth"
	"github.com/emdzej/swsrs/internal/discovery"
)

// fakeIdP serves a minimal OIDC discovery doc + JWKS so auth.NewVerifier
// is satisfied. The endpoints below mirror what we expect to be surfaced
// via the swsrs discovery handler.
func fakeIdP(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewUnstartedServer(mux)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                srv.URL,
			"authorization_endpoint":                srv.URL + "/auth",
			"token_endpoint":                        srv.URL + "/token",
			"device_authorization_endpoint":         srv.URL + "/device",
			"jwks_uri":                              srv.URL + "/jwks",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"keys":[]}`))
	})
	srv.Start()
	return srv
}

func TestDiscoveryHandler(t *testing.T) {
	idp := fakeIdP(t)
	defer idp.Close()

	v, err := auth.NewVerifier(context.Background(), idp.URL, "swsrs")
	if err != nil {
		t.Fatalf("verifier: %v", err)
	}

	h := discovery.Handler(v, []string{"swsrs:session:create"}, "swsrs-cli")
	req := httptest.NewRequest(http.MethodGet, "/.well-known/swsrs-config", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	var got discovery.Config
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Issuer != idp.URL {
		t.Errorf("issuer = %q, want %q", got.Issuer, idp.URL)
	}
	if got.Audience != "swsrs" {
		t.Errorf("audience = %q", got.Audience)
	}
	if got.DeviceAuthorizationEndpoint == "" || !strings.HasSuffix(got.DeviceAuthorizationEndpoint, "/device") {
		t.Errorf("device endpoint = %q", got.DeviceAuthorizationEndpoint)
	}
	if got.ClientIDHint != "swsrs-cli" {
		t.Errorf("client_id_hint = %q", got.ClientIDHint)
	}
	if len(got.Scopes) != 1 || got.Scopes[0] != "swsrs:session:create" {
		t.Errorf("scopes = %v", got.Scopes)
	}
}

func TestDiscoveryNoAuth(t *testing.T) {
	h := discovery.Handler(nil, nil, "")
	req := httptest.NewRequest(http.MethodGet, "/.well-known/swsrs-config", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 in --no-auth, got %d", rec.Code)
	}
}
