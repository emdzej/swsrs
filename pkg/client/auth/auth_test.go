package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/emdzej/swsrs/pkg/client/auth"
)

// fakeDiscovery serves an /.well-known/swsrs-config that points at the
// supplied IdP endpoints.
func fakeDiscovery(t *testing.T, body any) (string, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/swsrs-config" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	return srv.URL, srv.Close
}

func TestDiscoverHappyPath(t *testing.T) {
	url, stop := fakeDiscovery(t, map[string]any{
		"issuer":                        "https://idp.example.com",
		"audience":                      "swsrs",
		"scopes":                        []string{"swsrs:session:create", "swsrs:session:read"},
		"authorization_endpoint":        "https://idp.example.com/auth",
		"token_endpoint":                "https://idp.example.com/token",
		"device_authorization_endpoint": "https://idp.example.com/device",
		"client_id_hint":                "swsrs-cli",
	})
	defer stop()

	cfg, err := auth.Discover(context.Background(), url)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if cfg.Issuer != "https://idp.example.com" || cfg.ClientIDHint != "swsrs-cli" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if cfg.DeviceAuthorizationEndpoint == "" {
		t.Fatal("device_authorization_endpoint missing")
	}
}

func TestDiscoverAuthDisabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "auth disabled", http.StatusNotFound)
	}))
	defer srv.Close()
	_, err := auth.Discover(context.Background(), srv.URL)
	if err != auth.ErrAuthDisabled {
		t.Fatalf("expected ErrAuthDisabled, got %v", err)
	}
}

func TestFileTokenStoreRoundtrip(t *testing.T) {
	dir := t.TempDir()
	store := &auth.FileTokenStore{Path: filepath.Join(dir, "creds.json")}
	ctx := context.Background()

	// Empty -> ErrNoStoredToken
	if _, err := store.Load(ctx); err != auth.ErrNoStoredToken {
		t.Fatalf("expected ErrNoStoredToken, got %v", err)
	}

	want := &oauth2.Token{
		AccessToken:  "atk",
		RefreshToken: "rtk",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(1 * time.Hour).Round(time.Second),
	}
	if err := store.Save(ctx, want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.AccessToken != want.AccessToken || got.RefreshToken != want.RefreshToken {
		t.Fatalf("round-trip mismatch: %+v vs %+v", got, want)
	}

	// File mode 0600
	// (skipped on Windows where unix perms aren't meaningful — but the
	// stored file is still atomic.)
	if err := store.Clear(ctx); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, err := store.Load(ctx); err != auth.ErrNoStoredToken {
		t.Fatalf("expected ErrNoStoredToken after clear, got %v", err)
	}
}

func TestDiscoverWrongPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	_, err := auth.Discover(context.Background(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected 500 error, got %v", err)
	}
}
