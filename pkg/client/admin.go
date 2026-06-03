package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Session mirrors the admin API's create response.
type Session struct {
	ID             string    `json:"id"`
	InitiatorToken string    `json:"initiator_token"`
	ResponderToken string    `json:"responder_token"`
	InitiatorURL   string    `json:"initiator_url,omitempty"`
	ResponderURL   string    `json:"responder_url,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	ExpiresAt      time.Time `json:"expires_at"`
}

// SessionStatus mirrors the admin API's status response.
type SessionStatus struct {
	ID                  string    `json:"id"`
	State               string    `json:"state"`
	CreatedAt           time.Time `json:"created_at"`
	ExpiresAt           time.Time `json:"expires_at"`
	LastActivity        time.Time `json:"last_activity"`
	BytesIn             uint64    `json:"bytes_in"`
	BytesOut            uint64    `json:"bytes_out"`
	InitiatorConnected  bool      `json:"initiator_connected"`
	ResponderConnected  bool      `json:"responder_connected"`
}

// TokenSource provides a bearer token for admin calls. It is invoked per
// request so callers can rotate / refresh on each call.
type TokenSource func(ctx context.Context) (string, error)

// StaticToken returns a TokenSource that always yields the same token.
// Convenient for short-lived tools; production callers should pass a real
// refresh-aware source.
func StaticToken(token string) TokenSource {
	return func(context.Context) (string, error) { return token, nil }
}

// Admin is a client for /admin/sessions.
type Admin struct {
	// BaseURL is the relay's admin base, e.g. "https://relay.example.com".
	BaseURL string

	// Token is invoked to fetch a bearer token for each request.
	Token TokenSource

	// HTTPClient is optional; defaults to http.DefaultClient.
	HTTPClient *http.Client
}

func (a *Admin) httpClient() *http.Client {
	if a.HTTPClient != nil {
		return a.HTTPClient
	}
	return http.DefaultClient
}

// CreateSession allocates a new session and returns its tokens.
func (a *Admin) CreateSession(ctx context.Context) (*Session, error) {
	var s Session
	if err := a.do(ctx, http.MethodPost, "/admin/sessions", nil, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// GetSession returns the current status of a session.
func (a *Admin) GetSession(ctx context.Context, id string) (*SessionStatus, error) {
	var s SessionStatus
	if err := a.do(ctx, http.MethodGet, "/admin/sessions/"+id, nil, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// DeleteSession terminates a session.
func (a *Admin) DeleteSession(ctx context.Context, id string) error {
	return a.do(ctx, http.MethodDelete, "/admin/sessions/"+id, nil, nil)
}

// ListSessions returns all sessions visible to the caller.
func (a *Admin) ListSessions(ctx context.Context) ([]SessionStatus, error) {
	var resp struct {
		Sessions []SessionStatus `json:"sessions"`
	}
	if err := a.do(ctx, http.MethodGet, "/admin/sessions", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Sessions, nil
}

func (a *Admin) do(ctx context.Context, method, path string, in any, out any) error {
	if a.BaseURL == "" {
		return errors.New("client: Admin.BaseURL is required")
	}
	if a.Token == nil {
		return errors.New("client: Admin.Token is required")
	}

	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = strings.NewReader(string(b))
	}

	url := strings.TrimRight(a.BaseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}
	tok, err := a.Token(ctx)
	if err != nil {
		return fmt.Errorf("token source: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := a.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("admin %s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(msg)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
