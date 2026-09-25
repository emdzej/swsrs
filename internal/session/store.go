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
}

func NewStore(ttl time.Duration) *Store {
	return &Store{
		sessions: make(map[string]*Session),
		ttl:      ttl,
	}
}

func (s *Store) Create() *Session {
	sess := New(s.ttl)
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

func (s *Store) Delete(id string) bool {
	s.mu.Lock()
	sess, ok := s.sessions[id]
	if ok {
		delete(s.sessions, id)
	}
	s.mu.Unlock()
	if ok {
		sess.Close()
	}
	return ok
}

// Reap removes expired sessions. Intended to be called periodically.
func (s *Store) Reap(now time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for id, sess := range s.sessions {
		if now.After(sess.ExpiresAt) {
			sess.Close()
			delete(s.sessions, id)
			n++
		}
	}
	return n
}

// CloseAll closes and removes every session. Used on server shutdown.
func (s *Store) CloseAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, sess := range s.sessions {
		sess.Close()
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
