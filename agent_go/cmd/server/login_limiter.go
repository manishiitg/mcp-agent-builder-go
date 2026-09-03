package server

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// loginRateLimiter holds off an address after repeated failed logins. Small
// and in-memory on purpose (one server process): 10 failures within 10
// minutes lock the address for 10 minutes. A success clears its slate.
type loginRateLimiter struct {
	mu       sync.Mutex
	failures map[string]*loginFailures
	limit    int
	window   time.Duration
	lockout  time.Duration
	now      func() time.Time
}

type loginFailures struct {
	count       int
	windowStart time.Time
	lockedUntil time.Time
}

var loginLimiter = &loginRateLimiter{failures: map[string]*loginFailures{}, limit: 10, window: 10 * time.Minute, lockout: 10 * time.Minute, now: time.Now}

func (l *loginRateLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	f, ok := l.failures[ip]
	if !ok {
		return true
	}
	now := l.now()
	if now.Before(f.lockedUntil) {
		return false
	}
	if now.Sub(f.windowStart) > l.window {
		delete(l.failures, ip)
	}
	return true
}

func (l *loginRateLimiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	f, ok := l.failures[ip]
	if !ok || now.Sub(f.windowStart) > l.window {
		f = &loginFailures{windowStart: now}
		l.failures[ip] = f
	}
	f.count++
	if f.count >= l.limit {
		f.lockedUntil = now.Add(l.lockout)
		f.count = 0
		f.windowStart = now
	}
	// Keep the map bounded under a spray of addresses.
	if len(l.failures) > 10000 {
		for k, v := range l.failures {
			if now.Sub(v.windowStart) > l.window && now.After(v.lockedUntil) {
				delete(l.failures, k)
			}
		}
	}
}

func (l *loginRateLimiter) reset(ip string) {
	l.mu.Lock()
	delete(l.failures, ip)
	l.mu.Unlock()
}

// clientIPOf is the first X-Forwarded-For hop (Caddy and the gateway both
// sit in front of this server) or the socket address.
func clientIPOf(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
			return first
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
