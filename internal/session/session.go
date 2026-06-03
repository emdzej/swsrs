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

	mu             sync.Mutex
	state          State
	initiatorToken string
	responderToken string
	initiator      *peer
	responder      *peer
	bytesIn        uint64 // bytes from initiator to responder
	bytesOut       uint64 // bytes from responder to initiator
	lastActivity   time.Time
	closeOnce      sync.Once
	closed         chan struct{}
}

type peer struct {
	conn     *websocket.Conn
	role     Role
	attached time.Time
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func newToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

// New constructs a session with freshly generated id and per-slot tokens.
func New(ttl time.Duration) *Session {
	now := time.Now()
	return &Session{
		ID:             newID(),
		CreatedAt:      now,
		ExpiresAt:      now.Add(ttl),
		state:          StatePending,
		initiatorToken: newToken(),
		responderToken: newToken(),
		lastActivity:   now,
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
)

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

	p := &peer{conn: conn, role: role, attached: time.Now()}
	switch role {
	case RoleInitiator:
		if s.initiator != nil {
			return ErrSlotTaken
		}
		s.initiator = p
	case RoleResponder:
		if s.responder != nil {
			return ErrSlotTaken
		}
		s.responder = p
	}

	if s.initiator != nil && s.responder != nil {
		s.state = StateOpen
	} else {
		s.state = StateHalfOpen
	}
	s.lastActivity = time.Now()
	return nil
}

// Detach removes a peer from its slot. If both peers leave the session
// transitions to closed.
func (s *Session) Detach(role Role) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch role {
	case RoleInitiator:
		s.initiator = nil
	case RoleResponder:
		s.responder = nil
	}
	if s.initiator == nil && s.responder == nil {
		s.state = StatePending
	} else {
		s.state = StateHalfOpen
	}
	s.lastActivity = time.Now()
}

// Peer returns the connection of the *other* slot, if connected.
func (s *Session) Peer(self Role) *websocket.Conn {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch self {
	case RoleInitiator:
		if s.responder != nil {
			return s.responder.conn
		}
	case RoleResponder:
		if s.initiator != nil {
			return s.initiator.conn
		}
	}
	return nil
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

// Close marks the session terminated and closes the broadcast channel.
func (s *Session) Close() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.state = StateClosed
		s.mu.Unlock()
		close(s.closed)
	})
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
	}
}
