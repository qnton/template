package httpx

import (
	"testing"
	"time"
)

func TestRateLimiterBurstAndRefill(t *testing.T) {
	rl := NewRateLimiter(1, 2) // 1 token/sec, burst 2
	base := time.Unix(1_700_000_000, 0)
	rl.now = func() time.Time { return base }

	// Two separate calls (each consumes a token) so the burst of 2 is exhausted.
	first := rl.allow("a")
	second := rl.allow("a")
	if !first || !second {
		t.Fatal("first two requests should be allowed (burst = 2)")
	}
	if rl.allow("a") {
		t.Fatal("third immediate request should be denied")
	}

	// A different key has its own bucket.
	if !rl.allow("b") {
		t.Fatal("a separate key should have a fresh bucket")
	}

	// After 1 second, one token is refilled.
	rl.now = func() time.Time { return base.Add(time.Second) }
	if !rl.allow("a") {
		t.Fatal("after 1s a refilled token should allow the request")
	}
	if rl.allow("a") {
		t.Fatal("only one token should have refilled")
	}
}

func TestRateLimiterSweep(t *testing.T) {
	rl := NewRateLimiter(1, 1)
	base := time.Unix(1_700_000_000, 0)
	rl.now = func() time.Time { return base }
	rl.allow("stale")

	// Advance beyond the TTL and sweep; the idle bucket should be evicted.
	rl.now = func() time.Time { return base.Add(2 * rl.ttl) }
	rl.sweep()

	rl.mu.Lock()
	_, exists := rl.buckets["stale"]
	rl.mu.Unlock()
	if exists {
		t.Fatal("idle bucket should have been swept")
	}
}
