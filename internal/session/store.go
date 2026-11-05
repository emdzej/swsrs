package session

import (
	"context"
	"slices"
	"sync"
	"time"
)

// Store is an in-memory registry of active sessions.
// Safe for concurrent use.
type Store struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	ttl      time.Duration
	// Retention is how long a closed session's status stays readable (so a
	// control plane polling it sees how it ended, and its byte counts),
	// before the reaper removes it. 0: removed as it closes.
	Retention time.Duration
}

func NewStore(ttl time.Duration) *Store {
	return &Store{
		sessions: make(map[string]*Session),
		ttl:      ttl,
	}
}

// MaxTTL is the server's session lifetime: the default, and the most a
// session may ask for.
func (s *Store) MaxTTL() time.Duration { return s.ttl }

func (s *Store) Create() *Session { return s.CreateWith(Options{}) }

// CreateWith makes a session with its own options; a TTL of 0 (or over the
// server's) is the server's.
func (s *Store) CreateWith(o Options) *Session {
	if o.TTL <= 0 || o.TTL > s.ttl {
		o.TTL = s.ttl
	}
	sess := NewWith(o)
	s.mu.Lock()
	s.sessions[sess.ID] = sess
	s.mu.Unlock()
	return sess
}

func (s *Store) Get(id string) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	return sess, ok
}

func (s *Store) List() []*Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		out = append(out, sess)
	}
	slices.SortFunc(out, func(a, b *Session) int { return a.CreatedAt.Compare(b.CreatedAt) })
	return out
}

// Delete closes a session (peers disconnected, tokens useless). Its status
// stays readable for Retention.
func (s *Store) Delete(id string) bool {
	s.mu.Lock()
	sess, ok := s.sessions[id]
	if ok && s.Retention <= 0 {
		delete(s.sessions, id)
	}
	s.mu.Unlock()
	if ok {
		sess.CloseWith(ReasonDeleted)
	}
	return ok
}

// Reap closes expired sessions and removes those closed longer than
// Retention ago. Intended to be called periodically. Returns how many it
// removed.
func (s *Store) Reap(now time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for id, sess := range s.sessions {
		c := sess.ClosedAt()
		if c.IsZero() {
			if !now.After(sess.ExpiresAt) {
				continue // live
			}
			sess.CloseWith(ReasonExpired)
			if s.Retention > 0 {
				continue // readable for Retention from now
			}
		} else if now.Sub(c) < s.Retention {
			continue
		}
		delete(s.sessions, id)
		n++
	}
	return n
}

// CloseAll closes and removes every session. Used on server shutdown.
func (s *Store) CloseAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, sess := range s.sessions {
		sess.CloseWith(ReasonShutdown)
		delete(s.sessions, id)
	}
}

// RunReaper blocks, periodically reaping expired sessions until ctx is done.
func (s *Store) RunReaper(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			s.Reap(now)
		}
	}
}
