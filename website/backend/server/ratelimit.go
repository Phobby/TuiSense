//go:build linux

package server

import (
	"sync"
	"time"
)

// bucket is a token bucket: capacity tokens, refilled at rate per second.
type bucket struct {
	tokens float64
	last   time.Time
}

// rateLimiter keeps one token bucket per key (a client address).
type rateLimiter struct {
	mu       sync.Mutex
	capacity float64
	rate     float64 // tokens per second
	buckets  map[string]*bucket
	now      func() time.Time
}

func newRateLimiter(capacity int, per time.Duration) *rateLimiter {
	return &rateLimiter{capacity: float64(capacity), rate: float64(capacity) / per.Seconds(),
		buckets: map[string]*bucket{}, now: time.Now}
}

// Allow takes one token for key, reporting false when none is left.
func (r *rateLimiter) Allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	b, ok := r.buckets[key]
	if !ok {
		b = &bucket{tokens: r.capacity, last: now}
		r.buckets[key] = b
	}
	b.tokens = min(r.capacity, b.tokens+now.Sub(b.last).Seconds()*r.rate)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Prune forgets keys whose bucket is full again (no recent activity).
func (r *rateLimiter) Prune() {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	for k, b := range r.buckets {
		if b.tokens+now.Sub(b.last).Seconds()*r.rate >= r.capacity {
			delete(r.buckets, k)
		}
	}
}

// byteBudget limits a byte stream (one session's input) to rate bytes per
// second with a burst of twice that.
type byteBudget struct {
	rate, burst, tokens float64
	last                time.Time
}

func newByteBudget(perSecond int) *byteBudget {
	return &byteBudget{rate: float64(perSecond), burst: 2 * float64(perSecond), tokens: 2 * float64(perSecond), last: time.Now()}
}

// Take reports whether n bytes fit in the budget, consuming them if so.
func (b *byteBudget) Take(n int, now time.Time) bool {
	b.tokens = min(b.burst, b.tokens+now.Sub(b.last).Seconds()*b.rate)
	b.last = now
	if float64(n) > b.tokens {
		return false
	}
	b.tokens -= float64(n)
	return true
}
