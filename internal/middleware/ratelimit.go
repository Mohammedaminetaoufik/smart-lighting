package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimiter is a simple in-memory fixed-window limiter keyed by client IP.
// It is intended for endpoints like login to slow down credential brute-force.
// Note: in-memory only — per-process, resets on restart. Sufficient for a
// single-instance local/demo deployment; use Redis for multi-instance production.
type RateLimiter struct {
	mu       sync.Mutex
	hits     map[string][]time.Time
	max      int
	window   time.Duration
	lastGC   time.Time
}

// NewRateLimiter allows `max` requests per `window` per IP.
func NewRateLimiter(max int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		hits:   make(map[string][]time.Time),
		max:    max,
		window: window,
		lastGC: time.Now(),
	}
}

// Middleware returns a Gin handler enforcing the limit and aborting with 429.
func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		now := time.Now()

		rl.mu.Lock()
		// Occasional garbage collection of stale IP entries.
		if now.Sub(rl.lastGC) > 10*rl.window {
			for k, ts := range rl.hits {
				if len(ts) == 0 || now.Sub(ts[len(ts)-1]) > rl.window {
					delete(rl.hits, k)
				}
			}
			rl.lastGC = now
		}

		// Keep only timestamps within the current window.
		cutoff := now.Add(-rl.window)
		recent := rl.hits[ip][:0]
		for _, t := range rl.hits[ip] {
			if t.After(cutoff) {
				recent = append(recent, t)
			}
		}

		if len(recent) >= rl.max {
			rl.hits[ip] = recent
			rl.mu.Unlock()
			c.Header("Retry-After", "60")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "Trop de tentatives. Réessayez dans une minute.",
			})
			return
		}

		rl.hits[ip] = append(recent, now)
		rl.mu.Unlock()
		c.Next()
	}
}
