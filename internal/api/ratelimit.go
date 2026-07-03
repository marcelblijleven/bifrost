package api

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	loginMaxAttempts = 5
	loginWindow      = 5 * time.Minute
	loginLockout     = 5 * time.Minute
)

type loginBucket struct {
	failures    int
	windowStart time.Time
	lockedUntil time.Time
}

// loginLimiter throttles failed login attempts per email and per source IP to
// slow down credential-stuffing and brute-force attacks. State is in-memory
// and per-process — bifrost runs as a single binary, so this needs no
// external store.
type loginLimiter struct {
	mu      sync.Mutex
	buckets map[string]*loginBucket
}

func newLoginLimiter() *loginLimiter {
	l := &loginLimiter{buckets: make(map[string]*loginBucket)}
	go l.sweepLoop()
	return l
}

// locked reports whether key is currently locked out, and if so for how much longer.
func (l *loginLimiter) locked(key string) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	if !ok {
		return 0, false
	}
	if remaining := time.Until(b.lockedUntil); remaining > 0 {
		return remaining, true
	}
	return 0, false
}

// recordFailure increments key's failure count within the current window,
// locking it out once the count reaches loginMaxAttempts.
func (l *loginLimiter) recordFailure(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	b, ok := l.buckets[key]
	if !ok || now.Sub(b.windowStart) > loginWindow {
		b = &loginBucket{windowStart: now}
		l.buckets[key] = b
	}
	b.failures++
	if b.failures >= loginMaxAttempts {
		b.lockedUntil = now.Add(loginLockout)
	}
}

// reset clears key's failure history, e.g. after a successful login.
func (l *loginLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.buckets, key)
}

// sweepLoop periodically evicts buckets that are no longer relevant, so the
// map doesn't grow unbounded with one-off/expired entries over the life of
// the process.
func (l *loginLimiter) sweepLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		l.mu.Lock()
		now := time.Now()
		for k, b := range l.buckets {
			if now.Sub(b.windowStart) > loginWindow && now.After(b.lockedUntil) {
				delete(l.buckets, k)
			}
		}
		l.mu.Unlock()
	}
}

// clientIP extracts the request's remote IP, stripping the port.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func loginRateLimitKeys(r *http.Request, email string) (emailKey, ipKey string) {
	return "email:" + strings.ToLower(strings.TrimSpace(email)), "ip:" + clientIP(r)
}
