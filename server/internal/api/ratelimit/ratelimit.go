// Package ratelimit provides a fixed-window per-key rate limiter,
// intended for throttling repeated failed login attempts per client.
package ratelimit

import (
	"sync"
	"time"
)

// bucket is one key's failure count within its current window.
type bucket struct {
	count       int
	windowStart time.Time
}

// Limiter tracks failed attempts per key in fixed windows of length per.
// Only failures consume the allowance: call Hit to record a failed
// attempt and Allow to check whether the key is still within its count
// allowance. Successful actions never call Hit. A Limiter is safe for
// concurrent use.
type Limiter struct {
	mu      sync.Mutex
	count   int
	per     time.Duration
	now     func() time.Time
	buckets map[string]*bucket
}

// NewLimiter allows up to count failed attempts per key per window of
// length per. now is the time source, injectable for tests.
func NewLimiter(count int, per time.Duration, now func() time.Time) *Limiter {
	return &Limiter{
		count:   count,
		per:     per,
		now:     now,
		buckets: make(map[string]*bucket),
	}
}

// Allow reports whether key has fewer than the limit of recorded failures
// in the current window. It never records anything, so any number of
// successful actions leaves the allowance untouched.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[key]
	if !ok {
		return true
	}
	if l.now().Sub(b.windowStart) >= l.per {
		return true // window elapsed: allowance replenished
	}
	return b.count < l.count
}

// Hit records one failed attempt for key, opening a new window when the
// previous one has elapsed.
func (l *Limiter) Hit(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	b, ok := l.buckets[key]
	if !ok || now.Sub(b.windowStart) >= l.per {
		l.buckets[key] = &bucket{count: 1, windowStart: now}
		return
	}
	b.count++
}
