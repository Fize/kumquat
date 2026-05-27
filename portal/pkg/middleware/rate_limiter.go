package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimiter implements a simple token bucket rate limiter per client IP.
type RateLimiter struct {
	mu              sync.Mutex
	buckets         map[string]*tokenBucket
	rate            float64 // tokens per second
	burst           int     // max tokens (burst capacity)
	cleanupInterval time.Duration
}

type tokenBucket struct {
	tokens   float64
	lastTime time.Time
}

// NewRateLimiter creates a new rate limiter.
// rate: requests per second allowed.
// burst: maximum burst size.
func NewRateLimiter(rate float64, burst int) *RateLimiter {
	rl := &RateLimiter{
		buckets:         make(map[string]*tokenBucket),
		rate:            rate,
		burst:           burst,
		cleanupInterval: 10 * time.Minute,
	}
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.buckets[key]
	if !ok {
		b = &tokenBucket{
			tokens:   float64(rl.burst),
			lastTime: time.Now(),
		}
		rl.buckets[key] = b
		return true
	}

	now := time.Now()
	elapsed := now.Sub(b.lastTime).Seconds()
	b.tokens += elapsed * rl.rate
	if b.tokens > float64(rl.burst) {
		b.tokens = float64(rl.burst)
	}
	b.lastTime = now

	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true
	}
	return false
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		cutoff := time.Now().Add(-rl.cleanupInterval)
		for key, b := range rl.buckets {
			if b.lastTime.Before(cutoff) {
				delete(rl.buckets, key)
			}
		}
		rl.mu.Unlock()
	}
}

// RateLimit returns a Gin middleware that rate-limits requests based on client IP.
// rate: requests per second. burst: maximum burst size.
func RateLimit(rate float64, burst int) gin.HandlerFunc {
	limiter := NewRateLimiter(rate, burst)
	return func(c *gin.Context) {
		key := c.ClientIP()
		if !limiter.allow(key) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "rate limit exceeded",
				"message": "too many requests, please try again later",
			})
			return
		}
		c.Next()
	}
}

// RateLimitWithKey returns a Gin middleware that rate-limits requests using a custom key function.
func RateLimitWithKey(rate float64, burst int, keyFunc func(*gin.Context) string) gin.HandlerFunc {
	limiter := NewRateLimiter(rate, burst)
	return func(c *gin.Context) {
		key := keyFunc(c)
		if !limiter.allow(key) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "rate limit exceeded",
				"message": "too many requests, please try again later",
			})
			return
		}
		c.Next()
	}
}
