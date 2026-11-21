// Package session provides the in-memory store for authenticated
// connection sessions.
package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/dblama-dev/dblama/internal/driver"
)

// Session is one authenticated connection bound to its driver.
type Session struct {
	ID       string
	Driver   driver.Driver
	Database string
}

// sessionCtxKey is the context key type for a Session value; a distinct
// unexported type prevents collisions with keys from other packages.
type sessionCtxKey struct{}

// NewContext returns a copy of ctx carrying s. The auth middleware stores
// the authenticated session this way; handlers read it with FromContext.
func NewContext(ctx context.Context, s *Session) context.Context {
	return context.WithValue(ctx, sessionCtxKey{}, s)
}

// FromContext returns the Session stored in ctx, reporting whether one is
// present.
func FromContext(ctx context.Context) (*Session, bool) {
	s, ok := ctx.Value(sessionCtxKey{}).(*Session)
	return s, ok
}

// entry is a session in the store together with its last-use timestamp.
type entry struct {
	sess     *Session
	lastUsed time.Time
}

// Store keeps sessions in memory. All methods are safe for concurrent use.
type Store struct {
	mu          sync.Mutex
	ttl         time.Duration
	maxSessions int
	now         func() time.Time
	entries     map[string]*entry
}

// NewStore returns an empty store. Sessions idle longer than ttl expire;
// when creating a session would exceed maxSessions (> 0), the least
// recently used session is evicted. The now function is the time source,
// injectable for tests.
func NewStore(ttl time.Duration, maxSessions int, now func() time.Time) *Store {
	return &Store{
		ttl:         ttl,
		maxSessions: maxSessions,
		now:         now,
		entries:     make(map[string]*entry),
	}
}

// Create adds a new session with a random 256-bit hex ID and returns it.
// If the store is at maxSessions, the oldest idle session is evicted and
// its driver closed. database is "" when the login request had none.
func (s *Store) Create(d driver.Driver, database string) *Session {
	sess := &Session{ID: newID(), Driver: d, Database: database}
	now := s.now()

	s.mu.Lock()
	var evicted []driver.Driver
	if s.maxSessions > 0 {
		for len(s.entries) >= s.maxSessions {
			drv, ok := s.evictOldestLocked()
			if !ok {
				break
			}
			evicted = append(evicted, drv)
		}
	}
	s.entries[sess.ID] = &entry{sess: sess, lastUsed: now}
	s.mu.Unlock()

	for _, drv := range evicted {
		_ = drv.Close()
	}
	return sess
}

// Get returns the session with the given ID and renews its idle timer.
// A missing or expired session yields (nil, false); an expired session's
// driver is closed.
func (s *Store) Get(id string) (*Session, bool) {
	s.mu.Lock()
	e, ok := s.entries[id]
	if !ok {
		s.mu.Unlock()
		return nil, false
	}
	now := s.now()
	if now.Sub(e.lastUsed) > s.ttl {
		delete(s.entries, id)
		s.mu.Unlock()
		_ = e.sess.Driver.Close()
		return nil, false
	}
	e.lastUsed = now
	sess := e.sess
	s.mu.Unlock()
	return sess, true
}

// Delete removes the session with the given ID and closes its driver.
// Deleting an unknown ID is a no-op.
func (s *Store) Delete(id string) {
	s.mu.Lock()
	e, ok := s.entries[id]
	if !ok {
		s.mu.Unlock()
		return
	}
	delete(s.entries, id)
	s.mu.Unlock()
	_ = e.sess.Driver.Close()
}

// StartJanitor launches a background goroutine that closes sessions idle
// longer than the store's TTL, checking every interval using the
// store's time source. The returned stop function terminates the janitor
// and is safe to call more than once. A non-positive interval disables
// the janitor: time.NewTicker would panic, so no goroutine is started and
// the returned stop function is a no-op.
func (s *Store) StartJanitor(interval time.Duration) (stop func()) {
	if interval <= 0 {
		return func() {}
	}
	stopCh := make(chan struct{})
	var once sync.Once
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.sweep()
			case <-stopCh:
				return
			}
		}
	}()
	return func() {
		once.Do(func() { close(stopCh) })
	}
}

// sweep removes and closes every session idle longer than the TTL.
func (s *Store) sweep() {
	now := s.now()
	s.mu.Lock()
	var toClose []driver.Driver
	for id, e := range s.entries {
		if now.Sub(e.lastUsed) > s.ttl {
			delete(s.entries, id)
			toClose = append(toClose, e.sess.Driver)
		}
	}
	s.mu.Unlock()
	for _, drv := range toClose {
		_ = drv.Close()
	}
}

// evictOldestLocked removes the least recently used entry from the store
// and returns its driver. The store mutex must be held. It reports false
// when the store is empty.
func (s *Store) evictOldestLocked() (driver.Driver, bool) {
	var oldestID string
	var oldest *entry
	for id, e := range s.entries {
		if oldest == nil || e.lastUsed.Before(oldest.lastUsed) {
			oldestID, oldest = id, e
		}
	}
	if oldest == nil {
		return nil, false
	}
	delete(s.entries, oldestID)
	return oldest.sess.Driver, true
}

// newID returns a random 256-bit session ID as 64 hex characters.
func newID() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("session: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}
