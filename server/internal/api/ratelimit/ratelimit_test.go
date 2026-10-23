package ratelimit

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

// fakeClock is a controllable time source.
type fakeClock struct {
	mu  sync.Mutex
	cur time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cur
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cur = c.cur.Add(d)
}

func TestRateLimiter_TokenBucket(t *testing.T) {
	clk := &fakeClock{cur: time.Unix(0, 0)}
	per := time.Minute
	l := NewLimiter(10, per, clk.Now)
	const key = "1.2.3.4"

	// Ten failed attempts; the attempt before each failure is still allowed.
	for i := 0; i < 10; i++ {
		if !l.Allow(key) {
			t.Fatalf("Allow #%d = false, want true", i+1)
		}
		l.Hit(key)
	}
	if l.Allow(key) {
		t.Error("Allow after 10 hits = true, want false")
	}

	// After the window elapses the bucket replenishes.
	clk.Advance(per)
	if !l.Allow(key) {
		t.Error("Allow after window elapsed = false, want true")
	}
}

func TestRateLimiter_SuccessDoesNotHit(t *testing.T) {
	clk := &fakeClock{cur: time.Unix(0, 0)}
	l := NewLimiter(3, time.Minute, clk.Now)

	// Allow never records anything, so any number of successful checks
	// stays within the limit.
	for i := 0; i < 100; i++ {
		if !l.Allow("k") {
			t.Fatalf("Allow #%d = false, want true", i+1)
		}
	}
}

func TestRateLimiter_PrunesExpiredBuckets(t *testing.T) {
	clk := &fakeClock{cur: time.Unix(0, 0)}
	l := NewLimiter(3, time.Minute, clk.Now)

	// Fill well past the prune threshold with one window of failures.
	const keys = 2000
	for i := 0; i < keys; i++ {
		l.Hit(strconv.Itoa(i))
	}
	if got := l.Len(); got != keys {
		t.Fatalf("Len after %d hits = %d, want %d", keys, got, keys)
	}

	// After the window elapses the next Hit sweeps the expired buckets
	// away; only the fresh one remains.
	clk.Advance(2 * time.Minute)
	l.Hit("fresh")
	if got := l.Len(); got != 1 {
		t.Fatalf("Len after prune = %d, want 1", got)
	}
	if !l.Allow("0") {
		t.Error("Allow on pruned key = false, want true (bucket was expired)")
	}
}
