package auth

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"golang.org/x/oauth2"

	"github.com/emdzej/swsrs/pkg/client"
)

// AdminTokenSource returns a client.TokenSource compatible with
// client.Admin.Token. It:
//
//   - loads a cached token from the store
//   - returns the access token if still valid
//   - refreshes transparently when the IdP supplied a refresh_token
//   - persists the refreshed token back to the store
//   - refreshes with the client_id recorded at login (tok.Extra("client_id"),
//     set by DeviceLogin), falling back to cfg.ClientIDHint
//   - returns an error if no token is cached or the refresh fails — the
//     caller is expected to invoke `swsrs auth` (or another DeviceLogin)
//     and retry. The SDK does not silently re-prompt.
//
// cfg may be nil when the relay is running --no-auth; in that case the
// returned source always returns an empty token. (client.Admin will then
// send `Authorization: Bearer ` which the server ignores in --no-auth mode.)
func AdminTokenSource(cfg *Config, store TokenStore) client.TokenSource {
	if cfg == nil {
		return client.StaticToken("")
	}
	a := &adminSource{cfg: cfg, store: store}
	return a.token
}

type adminSource struct {
	cfg   *Config
	store TokenStore

	mu  sync.Mutex
	src oauth2.TokenSource
}

func (a *adminSource) token(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.src == nil {
		tok, err := a.store.Load(ctx)
		if err != nil {
			if errors.Is(err, ErrNoStoredToken) {
				return "", fmt.Errorf("auth: no cached token — run `swsrs auth` to log in")
			}
			return "", err
		}
		clientID, _ := tok.Extra("client_id").(string)
		if clientID == "" {
			clientID = a.cfg.ClientIDHint
		}
		oauthCfg := &oauth2.Config{
			ClientID: clientID,
			Endpoint: oauth2.Endpoint{TokenURL: a.cfg.TokenEndpoint},
		}
		// The token source keeps its context for every later refresh, so
		// it must not inherit this call's cancellation.
		a.src = &persistingSource{
			inner:    oauthCfg.TokenSource(context.WithoutCancel(ctx), tok),
			store:    a.store,
			clientID: clientID,
			last:     tok,
		}
	}
	tok, err := a.src.Token()
	if err != nil {
		return "", fmt.Errorf("auth: token unavailable — re-run `swsrs auth`: %w", err)
	}
	return tok.AccessToken, nil
}

// persistingSource wraps an oauth2.TokenSource and writes refreshed tokens
// back to the store whenever they change.
type persistingSource struct {
	inner    oauth2.TokenSource
	store    TokenStore
	clientID string
	last     *oauth2.Token
}

func (p *persistingSource) Token() (*oauth2.Token, error) {
	tok, err := p.inner.Token()
	if err != nil {
		return nil, err
	}
	if p.last == nil || tok.AccessToken != p.last.AccessToken || !tok.Expiry.Equal(p.last.Expiry) {
		_ = p.store.Save(context.Background(), withClientID(tok, p.clientID))
		p.last = tok
	}
	return tok, nil
}
