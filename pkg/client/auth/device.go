package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/oauth2"
)

// DevicePrompt is what the caller renders to the user during device flow.
type DevicePrompt struct {
	UserCode                string
	VerificationURI         string
	VerificationURIComplete string // optional; some IdPs supply a combined URL
	ExpiresAt               time.Time
	Interval                time.Duration
}

// DeviceLoginOptions configures a device-flow login.
type DeviceLoginOptions struct {
	// ClientID is the OAuth client_id registered at the IdP. Defaults to
	// Config.ClientIDHint.
	ClientID string

	// Scopes requested from the IdP. Defaults to Config.Scopes. Most IdPs
	// also need `openid offline_access` for refresh tokens; the SDK adds
	// these automatically unless OmitOpenIDScopes is set.
	Scopes []string

	// OnPrompt is invoked once the IdP returns a user_code; the caller
	// should display it (terminal, GUI, etc.) and tell the user to visit
	// the verification URL.
	OnPrompt func(DevicePrompt)

	// OmitOpenIDScopes disables auto-adding `openid` / `offline_access`.
	OmitOpenIDScopes bool
}

// DeviceLogin runs the device-authorization flow against the configured IdP
// and returns the resulting OAuth token. Blocks until the user completes
// authentication or the context / device-code expires.
func (c *Config) DeviceLogin(ctx context.Context, opts DeviceLoginOptions) (*oauth2.Token, error) {
	if c.DeviceAuthorizationEndpoint == "" {
		return nil, errors.New("auth: IdP does not advertise device_authorization_endpoint")
	}
	if c.TokenEndpoint == "" {
		return nil, errors.New("auth: discovery missing token_endpoint")
	}
	clientID := opts.ClientID
	if clientID == "" {
		clientID = c.ClientIDHint
	}
	if clientID == "" {
		return nil, errors.New("auth: no client_id supplied and discovery had no client_id_hint")
	}

	scopes := opts.Scopes
	if scopes == nil {
		scopes = append([]string{}, c.Scopes...)
	}
	if !opts.OmitOpenIDScopes {
		scopes = ensureScope(scopes, "openid")
		scopes = ensureScope(scopes, "offline_access")
	}

	oauthCfg := &oauth2.Config{
		ClientID: clientID,
		Endpoint: oauth2.Endpoint{
			AuthURL:       c.AuthorizationEndpoint,
			TokenURL:      c.TokenEndpoint,
			DeviceAuthURL: c.DeviceAuthorizationEndpoint,
		},
		Scopes: scopes,
	}

	da, err := oauthCfg.DeviceAuth(ctx)
	if err != nil {
		return nil, fmt.Errorf("auth: device authorization: %w", err)
	}
	if opts.OnPrompt != nil {
		opts.OnPrompt(DevicePrompt{
			UserCode:                da.UserCode,
			VerificationURI:         da.VerificationURI,
			VerificationURIComplete: da.VerificationURIComplete,
			ExpiresAt:               da.Expiry,
			Interval:                time.Duration(da.Interval) * time.Second,
		})
	}

	tok, err := oauthCfg.DeviceAccessToken(ctx, da)
	if err != nil {
		return nil, fmt.Errorf("auth: device token: %w", err)
	}
	return tok, nil
}

func ensureScope(scopes []string, want string) []string {
	for _, s := range scopes {
		if s == want {
			return scopes
		}
	}
	return append(scopes, want)
}
