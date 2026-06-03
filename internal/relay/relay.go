// Package relay implements the WebSocket data plane.
//
// The relay is protocol-agnostic — it forwards opaque WebSocket binary
// frames between the two peers of a session and inspects nothing.
package relay

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/emdzej/swsrs/internal/session"
)

// Handler serves the WS data-plane endpoint.
type Handler struct {
	Store  *session.Store
	Logger *slog.Logger
	// PeerWaitTimeout is how long a connected peer waits for its
	// counterpart before being disconnected.
	PeerWaitTimeout time.Duration
	// AllowedOrigins is passed to the WS accept options. Empty means
	// same-origin only.
	AllowedOrigins []string
	// MaxFrameSize is passed to (*websocket.Conn).SetReadLimit on every
	// accepted peer connection. -1 disables the limit (the right answer
	// for a protocol-agnostic relay forwarding opaque bytes). 0 is
	// treated as -1 for safety; positive values cap incoming frame size.
	MaxFrameSize int64
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /relay/{id}", h.serve)
}

func (h *Handler) serve(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := h.Store.Get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}

	token := tokenFromRequest(r)
	if token == "" {
		http.Error(w, "missing session token", http.StatusUnauthorized)
		return
	}
	role, err := sess.AuthorizeToken(token)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: h.AllowedOrigins,
	})
	if err != nil {
		h.Logger.Warn("ws accept failed", "session", id, "err", err)
		return
	}
	limit := h.MaxFrameSize
	if limit == 0 {
		limit = -1
	}
	conn.SetReadLimit(limit)
	// Background context — we manage lifetime explicitly below.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sess.Attach(role, conn); err != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, err.Error())
		return
	}
	defer sess.Detach(role)
	h.Logger.Info("peer attached", "session", id, "role", role)

	// Wait until the other peer arrives, the session closes, or we time out.
	peerConn, err := h.waitForPeer(ctx, sess, role)
	if err != nil {
		_ = conn.Close(websocket.StatusGoingAway, err.Error())
		h.Logger.Info("peer wait ended", "session", id, "role", role, "reason", err.Error())
		return
	}

	h.Logger.Info("relaying", "session", id, "role", role)
	h.pump(ctx, sess, role, conn, peerConn)
}

// waitForPeer blocks until the counterpart slot is connected, the session
// closes, or PeerWaitTimeout elapses.
func (h *Handler) waitForPeer(ctx context.Context, sess *session.Session, self session.Role) (*websocket.Conn, error) {
	deadline := time.NewTimer(h.PeerWaitTimeout)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		if c := sess.Peer(self); c != nil {
			return c, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-sess.Closed():
			return nil, errors.New("session closed")
		case <-deadline.C:
			return nil, errors.New("peer did not connect in time")
		case <-tick.C:
		}
	}
}

// pump forwards messages from `self` to `peer` until either side errors.
// The counterpart goroutine (the other peer's request handler) pumps the
// reverse direction.
func (h *Handler) pump(ctx context.Context, sess *session.Session, self session.Role, selfConn, peerConn *websocket.Conn) {
	for {
		typ, data, err := selfConn.Read(ctx)
		if err != nil {
			// Normal close — tell the peer to wind down.
			_ = peerConn.Close(websocket.StatusNormalClosure, "peer disconnected")
			return
		}
		if err := peerConn.Write(ctx, typ, data); err != nil {
			_ = selfConn.Close(websocket.StatusInternalError, "peer write failed")
			return
		}
		sess.AddBytes(self, len(data))
	}
}

// tokenFromRequest pulls a session token from Authorization: Bearer or
// ?token= query parameter. Browsers can't set headers on WS upgrades, so
// query-string is supported as a fallback.
func tokenFromRequest(r *http.Request) string {
	if h := r.Header.Get("Authorization"); len(h) > 7 && h[:7] == "Bearer " {
		return h[7:]
	}
	return r.URL.Query().Get("token")
}
