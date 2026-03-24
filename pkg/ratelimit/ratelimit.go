package ratelimit

import (
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Limiter provides a simple per-client-IP token bucket limiter.
type Limiter struct {
	enabled bool
	rps     int
	burst   int

	mu       sync.Mutex
	limiters map[string]*clientLimiter
}

type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func New(enabled bool, rps int, burst int) *Limiter {
	if rps <= 0 {
		enabled = false
	}
	if burst <= 0 {
		burst = 100
	}
	return &Limiter{
		enabled:  enabled,
		rps:      rps,
		burst:    burst,
		limiters: make(map[string]*clientLimiter),
	}
}

func (l *Limiter) Middleware(next http.Handler) http.Handler {
	if !l.enabled {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !l.allow(ip) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate limit exceeded"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (l *Limiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	entry, ok := l.limiters[ip]
	if !ok {
		entry = &clientLimiter{
			limiter:  rate.NewLimiter(rate.Limit(l.rps), l.burst),
			lastSeen: now,
		}
		l.limiters[ip] = entry
	}
	entry.lastSeen = now

	// Lightweight cleanup to avoid unbounded map growth.
	if len(l.limiters) > 10000 {
		cutoff := now.Add(-10 * time.Minute)
		for key, cl := range l.limiters {
			if cl.lastSeen.Before(cutoff) {
				delete(l.limiters, key)
			}
		}
	}

	return entry.limiter.Allow()
}

func clientIP(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}
	host := r.RemoteAddr
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == ':' {
			return host[:i]
		}
	}
	return host
}
