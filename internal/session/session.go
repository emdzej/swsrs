// Package session manages the lifecycle of relay sessions.
//
// A session is a two-slot rendezvous between an initiator and a responder.
// The relay is protocol-agnostic: it forwards opaque WebSocket binary frames
// between the two connected peers and nothing else.
package session

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type Role string

const (
	RoleInitiator Role = "initiator"
	RoleResponder Role = "responder"
)

type State int

const (
	StatePending  State = iota // created, no peers connected
	StateHalfOpen              // one peer connected, waiting for the other
	StateOpen                  // both peers connected, relaying
	StateClosed                // terminated
)

func (s State) String() string {
	switch s {
	case StatePending:
		return "pending"
	case StateHalfOpen:
		return "half_open"
	case StateOpen:
		return "open"
	case StateClosed:
		return "closed"
	}
	return "unknown"
}

type Session struct {
	ID        string
	CreatedAt time.Time
	ExpiresAt time.Time
	// Label is the control plane's own, opaque (e.g. which ticket the
	// session is for); returned in status, never interpreted.
	Label string
	// PeerWait overrides the relay's peer-wait for this session (0: the
	// relay's).
	PeerWait time.Duration
	// SingleAttach: each slot attaches once, and the session closes when
	// either peer leaves. For sessions a person ends by leaving, where a
	// token must not work again afterwards.
	SingleAttach bool

	mu             sync.Mutex
	state          State
	initiatorToken string
	responderToken string
	initiator      *peer
	responder      *peer
	bytesIn        uint64 // bytes from initiator to responder
	bytesOut       uint64 // bytes from responder to initiator
	lastActivity   time.Time
	initiatorAt    time.Time // first attach of each slot (zero: never)
	responderAt    time.Time
	openedAt       time.Time // first time both were attached
	closedAt       time.Time
	closeReason    string
	changed        chan struct{} // closed and replaced on every attach/detach
	closeOnce      sync.Once
	closed         chan struct{}
}

type peer struct {
	conn     *websocket.Conn
	role     Role
	attached time.Time
}

// random128 returns 128 random bits, base64url-encoded. Used for both
// session ids and slot tokens.
func random128() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

// Options are a session's own settings, given at creation. Zero values
// take the server's.
type Options struct {
	TTL          time.Duration
	PeerWait     time.Duration
	Label        string
	SingleAttach bool
}

// Close reasons, as status reports them.
const (
	ReasonDeleted  = "deleted"   // the admin API
	ReasonExpired  = "expired"   // its TTL passed
	ReasonPeerLeft = "peer_left" // single attach: a peer left
	ReasonShutdown = "shutdown"
)

// New constructs a session with freshly generated id and per-slot tokens.
func New(ttl time.Duration) *Session { return NewWith(Options{TTL: ttl}) }

// NewWith constructs a session with the given options (TTL required).
func NewWith(o Options) *Session {
	now := time.Now()
	return &Session{
		ID:             random128(),
		CreatedAt:      now,
		ExpiresAt:      now.Add(o.TTL),
		Label:          o.Label,
		PeerWait:       o.PeerWait,
		SingleAttach:   o.SingleAttach,
		state:          StatePending,
		initiatorToken: random128(),
		responderToken: random128(),
		lastActivity:   now,
		changed:        make(chan struct{}),
		closed:         make(chan struct{}),
	}
}

// Tokens returns the per-slot tokens. Caller MUST only return these in the
// admin response that created the session — never on lookup.
func (s *Session) Tokens() (initiator, responder string) {
	return s.initiatorToken, s.responderToken
}

// AuthorizeToken returns the role matching the supplied token, or an error.
// Uses constant-time comparison to avoid timing oracles.
func (s *Session) AuthorizeToken(token string) (Role, error) {
	if subtle.ConstantTimeCompare([]byte(token), []byte(s.initiatorToken)) == 1 {
		return RoleInitiator, nil
	}
	if subtle.ConstantTimeCompare([]byte(token), []byte(s.responderToken)) == 1 {
		return RoleResponder, nil
	}
	return "", ErrUnauthorized
}

var (
	ErrUnauthorized = errors.New("invalid session token")
	ErrSlotTaken    = errors.New("slot already connected")
	ErrExpired      = errors.New("session expired")
	ErrClosed       = errors.New("session closed")
	ErrSlotUsed     = errors.New("slot already used (single attach)")
)

// SlotFree reports whether the slot for role is currently unoccupied. The
// answer can be stale by the time the caller acts on it; Attach is the
// authoritative check.
func (s *Session) SlotFree(role Role) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if role == RoleInitiator {
		return s.initiator == nil && !(s.SingleAttach && !s.initiatorAt.IsZero())
	}
	return s.responder == nil && !(s.SingleAttach && !s.responderAt.IsZero())
}

// Attach binds a connection to a slot. Returns an error if the slot is taken
// or the session is closed/expired.
func (s *Session) Attach(role Role, conn *websocket.Conn) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == StateClosed {
		return ErrClosed
	}
	if time.Now().After(s.ExpiresAt) {
		return ErrExpired
	}

	now := time.Now()
	p := &peer{conn: conn, role: role, attached: now}
	switch role {
	case RoleInitiator:
		if s.initiator != nil {
			return ErrSlotTaken
		}
		if s.SingleAttach && !s.initiatorAt.IsZero() {
			return ErrSlotUsed
		}
		s.initiator = p
		if s.initiatorAt.IsZero() {
			s.initiatorAt = now
		}
	case RoleResponder:
		if s.responder != nil {
			return ErrSlotTaken
		}
		if s.SingleAttach && !s.responderAt.IsZero() {
			return ErrSlotUsed
		}
		s.responder = p
		if s.responderAt.IsZero() {
			s.responderAt = now
		}
	}

	if s.initiator != nil && s.responder != nil {
		s.state = StateOpen
		if s.openedAt.IsZero() {
			s.openedAt = now
		}
	} else {
		s.state = StateHalfOpen
	}
	s.lastActivity = time.Now()
	s.notifyLocked()
	return nil
}

// Detach removes a peer from its slot. When both slots are empty the
// session returns to pending, so peers can redial with the same tokens
// until the TTL expires. A closed session stays closed.
func (s *Session) Detach(role Role) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch role {
	case RoleInitiator:
		s.initiator = nil
	case RoleResponder:
		s.responder = nil
	}
	switch {
	case s.state == StateClosed:
	case s.initiator == nil && s.responder == nil:
		s.state = StatePending
	default:
		s.state = StateHalfOpen
	}
	s.lastActivity = time.Now()
	s.notifyLocked()
	if s.SingleAttach && s.state != StateClosed {
		s.closeLocked(ReasonPeerLeft)
	}
}

func (s *Session) notifyLocked() {
	close(s.changed)
	s.changed = make(chan struct{})
}

// Peer returns the connection of the *other* slot (nil if not connected)
// and a channel that is closed on the next attach or detach, so callers
// can wait for the counterpart without polling.
func (s *Session) Peer(self Role) (*websocket.Conn, <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var p *peer
	if self == RoleInitiator {
		p = s.responder
	} else {
		p = s.initiator
	}
	if p == nil {
		return nil, s.changed
	}
	return p.conn, s.changed
}

// AddBytes records relayed bytes for status reporting.
func (s *Session) AddBytes(from Role, n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if from == RoleInitiator {
		s.bytesIn += uint64(n)
	} else {
		s.bytesOut += uint64(n)
	}
	s.lastActivity = time.Now()
}

// Close marks the session terminated and closes the Closed channel. The
// relay handler watches that channel and disconnects attached peers.
func (s *Session) Close() { s.CloseWith(ReasonDeleted) }

// CloseWith closes the session, recording why (the first reason wins).
func (s *Session) CloseWith(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeLocked(reason)
}

func (s *Session) closeLocked(reason string) {
	s.closeOnce.Do(func() {
		s.state = StateClosed
		s.closedAt = time.Now()
		s.closeReason = reason
		close(s.closed)
	})
}

// ClosedAt is when the session closed (zero: still open).
func (s *Session) ClosedAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closedAt
}

// Closed returns a channel that fires when the session is terminated.
func (s *Session) Closed() <-chan struct{} { return s.closed }

// Status is the JSON-safe view of session state for the admin API.
type Status struct {
	ID           string    `json:"id"`
	State        string    `json:"state"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	LastActivity time.Time `json:"last_activity"`
	BytesIn      uint64    `json:"bytes_in"`
	BytesOut     uint64    `json:"bytes_out"`
	Initiator    bool      `json:"initiator_connected"`
	Responder    bool      `json:"responder_connected"`
	Label        string    `json:"label,omitempty"`
	SingleAttach bool      `json:"single_attach,omitempty"`
	PeerWaitSec  int64     `json:"peer_wait_seconds,omitempty"`
	// Lifecycle, for a control plane polling it: first attach of each
	// slot, when both first were, and when and why it closed.
	InitiatorAt *time.Time `json:"initiator_attached_at,omitempty"`
	ResponderAt *time.Time `json:"responder_attached_at,omitempty"`
	OpenedAt    *time.Time `json:"opened_at,omitempty"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
	CloseReason string     `json:"close_reason,omitempty"`
}

func timeOrNil(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func (s *Session) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Status{
		ID:           s.ID,
		State:        s.state.String(),
		CreatedAt:    s.CreatedAt,
		ExpiresAt:    s.ExpiresAt,
		LastActivity: s.lastActivity,
		BytesIn:      s.bytesIn,
		BytesOut:     s.bytesOut,
		Initiator:    s.initiator != nil,
		Responder:    s.responder != nil,
		Label:        s.Label,
		SingleAttach: s.SingleAttach,
		PeerWaitSec:  int64(s.PeerWait / time.Second),
		InitiatorAt:  timeOrNil(s.initiatorAt),
		ResponderAt:  timeOrNil(s.responderAt),
		OpenedAt:     timeOrNil(s.openedAt),
		ClosedAt:     timeOrNil(s.closedAt),
		CloseReason:  s.closeReason,
	}
}
