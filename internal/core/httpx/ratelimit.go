package httpx

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// RateLimiter is a per-key token-bucket limiter built on the standard library
// only — no golang.org/x/time/rate dependency, keeping the runtime module set
// minimal (swap in x/time/rate here if you prefer). It is DEFENSE-IN-DEPTH: the
// primary rate-limiting / WAF layer belongs at the Cloudflare edge.
//
// Keys are the real client IP (see RealIP). A background sweeper evicts idle
// buckets so memory stays bounded under churn.
type RateLimiter struct {
	rps   float64
	burst float64
	ttl   time.Duration

	mu      sync.Mutex
	buckets map[string]*tokenBucket
	now     func() time.Time // injectable for tests
}

type tokenBucket struct {
	tokens float64
	last   time.Time
	seen   time.Time
}

// NewRateLimiter creates a limiter allowing rps tokens/sec with the given burst.
func NewRateLimiter(rps float64, burst int) *RateLimiter {
	return &RateLimiter{
		rps:     rps,
		burst:   float64(burst),
		ttl:     10 * time.Minute,
		buckets: make(map[string]*tokenBucket),
		now:     time.Now,
	}
}

// allow consumes one token for key, refilling continuously since the last hit.
func (rl *RateLimiter) allow(key string) bool {
	now := rl.now()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	b := rl.buckets[key]
	if b == nil {
		b = &tokenBucket{tokens: rl.burst, last: now}
		rl.buckets[key] = b
	}
	b.tokens = min(rl.burst, b.tokens+now.Sub(b.last).Seconds()*rl.rps)
	b.last = now
	b.seen = now

	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// Middleware enforces the limit, returning 429 with Retry-After when exceeded.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := ClientIP(r.Context())
		if key == "" {
			key = r.RemoteAddr
		}
		if !rl.allow(key) {
			w.Header().Set("Retry-After", "1")
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// StartSweeper launches a goroutine that evicts idle buckets until ctx is done.
func (rl *RateLimiter) StartSweeper(ctx context.Context) {
	go func() {
		t := time.NewTicker(rl.ttl)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				rl.sweep()
			}
		}
	}()
}

func (rl *RateLimiter) sweep() {
	cutoff := rl.now().Add(-rl.ttl)
	rl.mu.Lock()
	defer rl.mu.Unlock()
	for k, b := range rl.buckets {
		if b.seen.Before(cutoff) {
			delete(rl.buckets, k)
		}
	}
}
