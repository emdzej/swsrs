// Package client is the Go SDK for the swsrs relay.
//
// Two APIs:
//   - Admin: create/list/delete sessions via the relay's HTTP admin API.
//   - Peer: Dial / Accept return a net.Conn over the relay WebSocket so
//     callers can run gRPC, HTTP, TLS, or any other byte-stream protocol
//     through the rendezvous without knowing it's there.
//
// The peer connection also exposes Send / Recv methods that preserve
// WebSocket message boundaries — use these for datagram-style traffic.
package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// DefaultKeepalive is how often the SDK sends WS pings to detect dead peers.
const DefaultKeepalive = 30 * time.Second

// DialOptions configures a peer connection.
type DialOptions struct {
	// RelayURL is the base URL of the relay, e.g. "wss://relay.example.com".
	// Both "ws://" / "wss://" and "http://" / "https://" are accepted; the
	// scheme is normalized for the WebSocket upgrade.
	RelayURL string

	// SessionID and Token come from the Admin API response.
	SessionID string
	Token     string

	// HTTPClient lets callers customize the underlying transport (proxies,
	// custom TLS roots). Optional.
	HTTPClient *http.Client

	// Keepalive interval. Zero means DefaultKeepalive; negative disables.
	Keepalive time.Duration

	// HandshakeTimeout bounds the WS upgrade. Zero means 10s.
	HandshakeTimeout time.Duration
}

// Dial connects to the relay as the initiator role. The returned Conn is a
// fully functional net.Conn; reads block until the peer attaches and sends
// data, writes succeed as soon as the peer is connected.
func Dial(ctx context.Context, opts DialOptions) (*Conn, error) {
	return connect(ctx, opts, "initiator")
}

// Accept connects to the relay as the responder role. Wire-identical to
// Dial — the name expresses caller intent.
func Accept(ctx context.Context, opts DialOptions) (*Conn, error) {
	return connect(ctx, opts, "responder")
}

func connect(ctx context.Context, opts DialOptions, role string) (*Conn, error) {
	if opts.RelayURL == "" || opts.SessionID == "" || opts.Token == "" {
		return nil, errors.New("client: RelayURL, SessionID, and Token are required")
	}
	wsURL, err := buildRelayURL(opts.RelayURL, opts.SessionID)
	if err != nil {
		return nil, err
	}

	timeout := opts.HandshakeTimeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	hsCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	ws, _, err := websocket.Dial(hsCtx, wsURL, &websocket.DialOptions{
		HTTPClient: httpClient,
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer " + opts.Token},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("client: ws dial: %w", err)
	}
	// Treat any frame size as legitimate — the relay forwards opaque bytes.
	ws.SetReadLimit(-1)

	c := &Conn{
		ws:      ws,
		role:    role,
		closed:  make(chan struct{}),
		localA:  syntheticAddr{kind: "swsrs:local:" + role},
		remoteA: syntheticAddr{kind: "swsrs:relay:" + opts.SessionID},
	}

	keepalive := opts.Keepalive
	if keepalive == 0 {
		keepalive = DefaultKeepalive
	}
	if keepalive > 0 {
		go c.runKeepalive(keepalive)
	}
	return c, nil
}

// buildRelayURL normalizes schemes and appends /relay/{id}. Accepts http(s)
// and ws(s) inputs, with or without a trailing slash.
func buildRelayURL(base, sessionID string) (string, error) {
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil {
		return "", fmt.Errorf("client: invalid RelayURL: %w", err)
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
		// fine
	default:
		return "", fmt.Errorf("client: unsupported scheme %q", u.Scheme)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/relay/" + sessionID
	return u.String(), nil
}

// Conn is a peer connection to the relay. Safe for one reader and one writer
// goroutine concurrently — the same shape as net.Conn.
type Conn struct {
	ws   *websocket.Conn
	role string

	readBuf []byte // leftover bytes from a partially-consumed frame

	readDeadline  atomic.Pointer[time.Time]
	writeDeadline atomic.Pointer[time.Time]

	localA, remoteA net.Addr

	closeOnce sync.Once
	closed    chan struct{}
	closeErr  error
}

// --- net.Conn ---

// Read pulls bytes from the next available WS message, collapsing frame
// boundaries into a byte stream (TCP-like view). Use Recv if you need
// boundary preservation.
func (c *Conn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(c.readBuf) > 0 {
		n := copy(p, c.readBuf)
		c.readBuf = c.readBuf[n:]
		return n, nil
	}
	ctx, cancel := c.contextWithDeadline(c.readDeadline.Load())
	defer cancel()
	_, data, err := c.ws.Read(ctx)
	if err != nil {
		return 0, normalizeErr(err)
	}
	n := copy(p, data)
	if n < len(data) {
		c.readBuf = data[n:]
	}
	return n, nil
}

// Write sends p as a single WS binary frame. Writes are atomic at the
// frame level; callers framing on top should match peer expectations.
func (c *Conn) Write(p []byte) (int, error) {
	ctx, cancel := c.contextWithDeadline(c.writeDeadline.Load())
	defer cancel()
	if err := c.ws.Write(ctx, websocket.MessageBinary, p); err != nil {
		return 0, normalizeErr(err)
	}
	return len(p), nil
}

// Close closes the relay connection. Idempotent.
func (c *Conn) Close() error {
	c.closeOnce.Do(func() {
		c.closeErr = c.ws.Close(websocket.StatusNormalClosure, "")
		close(c.closed)
	})
	return c.closeErr
}

func (c *Conn) LocalAddr() net.Addr  { return c.localA }
func (c *Conn) RemoteAddr() net.Addr { return c.remoteA }

func (c *Conn) SetDeadline(t time.Time) error {
	c.SetReadDeadline(t)
	c.SetWriteDeadline(t)
	return nil
}
func (c *Conn) SetReadDeadline(t time.Time) error {
	c.readDeadline.Store(&t)
	return nil
}
func (c *Conn) SetWriteDeadline(t time.Time) error {
	c.writeDeadline.Store(&t)
	return nil
}

// --- frame-preserving view ---

// Send writes msg as exactly one WS binary message. Pairs with Recv.
func (c *Conn) Send(ctx context.Context, msg []byte) error {
	return c.ws.Write(ctx, websocket.MessageBinary, msg)
}

// Recv returns exactly one WS message (binary or text). Pairs with Send.
// Bypasses any buffer Read might have left behind, so don't mix Recv with
// Read on the same logical stream.
func (c *Conn) Recv(ctx context.Context) ([]byte, error) {
	_, data, err := c.ws.Read(ctx)
	return data, normalizeErr(err)
}

// --- internals ---

func (c *Conn) runKeepalive(interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-c.closed:
			return
		case <-t.C:
			ctx, cancel := context.WithTimeout(context.Background(), interval)
			err := c.ws.Ping(ctx)
			cancel()
			if err != nil {
				_ = c.Close()
				return
			}
		}
	}
}

func (c *Conn) contextWithDeadline(d *time.Time) (context.Context, context.CancelFunc) {
	if d == nil || d.IsZero() {
		return context.WithCancel(context.Background())
	}
	return context.WithDeadline(context.Background(), *d)
}

// normalizeErr translates WS close errors into io.EOF where appropriate so
// callers using net.Conn semantics see clean stream termination.
func normalizeErr(err error) error {
	if err == nil {
		return nil
	}
	cs := websocket.CloseStatus(err)
	if cs == websocket.StatusNormalClosure || cs == websocket.StatusGoingAway {
		return io.EOF
	}
	return err
}

type syntheticAddr struct{ kind string }

func (s syntheticAddr) Network() string { return "swsrs" }
func (s syntheticAddr) String() string  { return s.kind }
