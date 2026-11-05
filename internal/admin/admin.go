// Package admin exposes session lifecycle endpoints, gated by OIDC scopes.
package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/emdzej/swsrs/internal/auth"
	"github.com/emdzej/swsrs/internal/session"
)

const (
	ScopeCreate = "swsrs:session:create"
	ScopeRead   = "swsrs:session:read"
	ScopeDelete = "swsrs:session:delete"
)

type API struct {
	Store    *session.Store
	Verifier *auth.Verifier
	// PublicBaseURL is the wss:// (or ws://) base used to build connect URLs
	// in the create response, e.g. "wss://relay.example.com". Optional.
	PublicBaseURL string
	// MaxPeerWait is the most a session may ask for as its peer wait (the
	// server's --peer-wait).
	MaxPeerWait time.Duration
}

// createRequest is the optional body of POST /admin/sessions. Zero values
// take the server's.
type createRequest struct {
	TTLSeconds      int64  `json:"ttl_seconds"`
	PeerWaitSeconds int64  `json:"peer_wait_seconds"`
	Label           string `json:"label"`
	SingleAttach    bool   `json:"single_attach"`
}

const maxLabel = 256

// Register wires admin routes onto the given mux. Scopes are enforced per
// route via the auth middleware. If Verifier is nil, all routes are
// unauthenticated — intended only for local development (--no-auth).
func (a *API) Register(mux *http.ServeMux) {
	mux.Handle("POST /admin/sessions", a.guard(ScopeCreate, http.HandlerFunc(a.create)))
	mux.Handle("GET /admin/sessions", a.guard(ScopeRead, http.HandlerFunc(a.list)))
	mux.Handle("GET /admin/sessions/{id}", a.guard(ScopeRead, http.HandlerFunc(a.get)))
	mux.Handle("DELETE /admin/sessions/{id}", a.guard(ScopeDelete, http.HandlerFunc(a.del)))
}

func (a *API) guard(scope string, h http.Handler) http.Handler {
	if a.Verifier == nil {
		return h
	}
	return a.Verifier.Middleware(scope, h)
}

type createResponse struct {
	ID             string    `json:"id"`
	InitiatorToken string    `json:"initiator_token"`
	ResponderToken string    `json:"responder_token"`
	InitiatorURL   string    `json:"initiator_url,omitempty"`
	ResponderURL   string    `json:"responder_url,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	ExpiresAt      time.Time `json:"expires_at"`
	Label          string    `json:"label,omitempty"`
	SingleAttach   bool      `json:"single_attach,omitempty"`
}

func (a *API) create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if r.ContentLength != 0 {
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "body: "+err.Error())
			return
		}
	}
	ttl, wait := time.Duration(req.TTLSeconds)*time.Second, time.Duration(req.PeerWaitSeconds)*time.Second
	switch {
	case req.TTLSeconds < 0 || ttl > a.Store.MaxTTL():
		writeError(w, http.StatusBadRequest, fmt.Sprintf("ttl_seconds: 1 to %d", int64(a.Store.MaxTTL()/time.Second)))
		return
	case req.PeerWaitSeconds < 0 || (a.MaxPeerWait > 0 && wait > a.MaxPeerWait):
		writeError(w, http.StatusBadRequest, fmt.Sprintf("peer_wait_seconds: 1 to %d", int64(a.MaxPeerWait/time.Second)))
		return
	case len(req.Label) > maxLabel:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("label: %d bytes at most", maxLabel))
		return
	}
	sess := a.Store.CreateWith(session.Options{TTL: ttl, PeerWait: wait, Label: req.Label, SingleAttach: req.SingleAttach})
	init, resp := sess.Tokens()
	out := createResponse{
		ID:             sess.ID,
		InitiatorToken: init,
		ResponderToken: resp,
		CreatedAt:      sess.CreatedAt,
		ExpiresAt:      sess.ExpiresAt,
		Label:          sess.Label,
		SingleAttach:   sess.SingleAttach,
	}
	if a.PublicBaseURL != "" {
		out.InitiatorURL = a.PublicBaseURL + "/relay/" + sess.ID
		out.ResponderURL = a.PublicBaseURL + "/relay/" + sess.ID
	}
	writeJSON(w, http.StatusCreated, out)
}

func (a *API) list(w http.ResponseWriter, r *http.Request) {
	sessions := a.Store.List()
	out := make([]session.Status, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, s.Status())
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
}

func (a *API) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := a.Store.Get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, sess.Status())
}

func (a *API) del(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !a.Store.Delete(id) {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
