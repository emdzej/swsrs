// Package relay implements the WebSocket data plane.
//
// The relay is protocol-agnostic — it forwards opaque WebSocket binary
// frames between the two peers of a session and inspects nothing.
package relay

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
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

	active sync.WaitGroup
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /relay/{id}", h.serve)
}

// Wait blocks until every relayed connection has finished or ctx is done.
// Hijacked WebSocket connections are not tracked by http.Server.Shutdown,
// so the server calls this after closing all sessions.
func (h *Handler) Wait(ctx context.Context) {
	done := make(chan struct{})
	go func() { h.active.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
	}
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
	// Pre-upgrade check so a busy slot is a plain HTTP 409 rather than a
	// close frame after the upgrade. Attach below remains authoritative.
	if !sess.SlotFree(role) {
		http.Error(w, session.ErrSlotTaken.Error(), http.StatusConflict)
		return
	}

	// Add before the upgrade: until the connection is hijacked,
	// http.Server.Shutdown still waits for this handler, so the Add
	// always happens before Wait is called.
	h.active.Add(1)
	defer h.active.Done()
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

	if err := sess.Attach(role, conn); err != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, err.Error())
		return
	}
	defer sess.Detach(role)
	h.Logger.Info("peer attached", "session", id, "role", role)

	// Background context — the request context is not usable after the
	// upgrade. watch and pump share it; either side ending cancels it.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		h.watch(ctx, sess, role, conn)
		cancel()
	}()

	err = h.pump(ctx, sess, role, conn)
	h.Logger.Info("peer detached", "session", id, "role", role, "reason", err)
}

// watch disconnects conn when the session is closed (admin delete, TTL
// reap, shutdown) or when the counterpart has not attached within
// PeerWaitTimeout. Returns once it has closed conn or ctx is done.
func (h *Handler) watch(ctx context.Context, sess *session.Session, self session.Role, conn *websocket.Conn) {
	timeout := time.NewTimer(h.PeerWaitTimeout)
	defer timeout.Stop()
	for {
		peer, changed := sess.Peer(self)
		if peer != nil {
			timeout.Stop()
		}
		select {
		case <-ctx.Done():
			return
		case <-sess.Closed():
			_ = conn.Close(websocket.StatusGoingAway, "session closed")
			return
		case <-timeout.C:
			_ = conn.Close(websocket.StatusGoingAway, "peer did not connect in time")
			return
		case <-changed:
		}
	}
}

// pump streams messages from selfConn to the counterpart until selfConn
// fails. The counterpart's own handler pumps the reverse direction.
//
// Reading starts before the counterpart attaches, so ping and close frames
// are answered while waiting. A data message that arrives early is held
// (unread) until the counterpart is there. Messages are streamed with
// io.Copy rather than buffered, so memory per connection stays bounded
// whatever MaxFrameSize is.
func (h *Handler) pump(ctx context.Context, sess *session.Session, self session.Role, selfConn *websocket.Conn) error {
	for {
		typ, r, err := selfConn.Reader(ctx)
		if err != nil {
			peerConn, _ := sess.Peer(self)
			select {
			case <-sess.Closed():
				// The counterpart's own watch closes it with the real reason.
			default:
				if peerConn != nil {
					_ = peerConn.Close(websocket.StatusNormalClosure, "peer disconnected")
				}
			}
			return err
		}
		peerConn, err := awaitPeer(ctx, sess, self)
		if err != nil {
			return err
		}
		n, err := forward(ctx, peerConn, typ, r)
		sess.AddBytes(self, int(n))
		if err != nil {
			// The counterpart may have seen part of a message; don't let
			// it mistake the prefix for a whole one.
			_ = peerConn.Close(websocket.StatusInternalError, "relay stream interrupted")
			_ = selfConn.Close(websocket.StatusInternalError, "relay stream interrupted")
			return err
		}
	}
}

func awaitPeer(ctx context.Context, sess *session.Session, self session.Role) (*websocket.Conn, error) {
	for {
		peerConn, changed := sess.Peer(self)
		if peerConn != nil {
			return peerConn, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-changed:
		}
	}
}

func forward(ctx context.Context, dst *websocket.Conn, typ websocket.MessageType, src io.Reader) (int64, error) {
	w, err := dst.Writer(ctx, typ)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(w, src)
	if err != nil {
		return n, err
	}
	return n, w.Close()
}

// tokenFromRequest pulls a session token from Authorization: Bearer or
// ?token= query parameter. Browsers can't set headers on WS upgrades, so
// query-string is supported as a fallback.
func tokenFromRequest(r *http.Request) string {
	if h := r.Header.Get("Authorization"); len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return h[7:]
	}
	return r.URL.Query().Get("token")
}
