// Package auth helps client apps obtain an OIDC token for a swsrs relay's
// admin API without hard-coding the IdP details.
//
// The flow:
//
//  1. Call [Discover] with the relay's base URL to read its public
//     `/.well-known/swsrs-config` document.
//  2. Use [Config.DeviceLogin] to run the OAuth 2.0 device-authorization
//     flow against the IdP. The caller renders the user_code +
//     verification URL however they like (terminal print, GUI dialog, QR).
//  3. Persist the token via a [TokenStore]. [FileTokenStore] gives you
//     a JSON file under the OS user-config directory.
//  4. Wire it into [client.Admin] using [AdminTokenSource] — the returned
//     source refreshes transparently and errors when re-login is needed.
//
// Browser apps should NOT use device flow (the token endpoint isn't
// CORS-enabled at most IdPs). For browser auth, run your own auth-code +
// PKCE flow and pass the access token to [client.Admin] directly.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Config is the parsed /.well-known/swsrs-config document.
type Config struct {
	Issuer                      string   `json:"issuer"`
	Audience                    string   `json:"audience"`
	Scopes                      []string `json:"scopes"`
	AuthorizationEndpoint       string   `json:"authorization_endpoint"`
	TokenEndpoint               string   `json:"token_endpoint"`
	DeviceAuthorizationEndpoint string   `json:"device_authorization_endpoint"`
	ClientIDHint                string   `json:"client_id_hint"`
}

// Discover fetches the relay's public discovery document.
func Discover(ctx context.Context, relayURL string) (*Config, error) {
	if relayURL == "" {
		return nil, errors.New("auth: relayURL is required")
	}
	url := strings.TrimRight(relayURL, "/") + "/.well-known/swsrs-config"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("auth: discovery: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrAuthDisabled
	}
	if resp.StatusCode >= 400 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("auth: discovery %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	var cfg Config
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("auth: decode discovery: %w", err)
	}
	return &cfg, nil
}

// ErrAuthDisabled is returned when the server replies 404 to discovery —
// it's running with --no-auth and no token is needed.
var ErrAuthDisabled = errors.New("auth: relay is running with auth disabled (no token needed)")
