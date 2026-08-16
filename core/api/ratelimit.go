package api

import (
	"net"
	"sync"
	"time"
)

// Login throttling: bcrypt already makes each attempt slow, but nothing
// else stops a scripted credential-stuffing run if the instance is ever
// reachable beyond the LAN. A fixed window per client IP and per username
// is enough friction for this threat model — no external deps, resets on
// successful login.
const (
	maxLoginAttempts = 10
	loginWindow      = time.Minute
)

type loginBucket struct {
	count   int
	resetAt time.Time
}

type loginLimiter struct {
	mu      sync.Mutex
	buckets map[string]*loginBucket
	now     func() time.Time // swappable for tests
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{buckets: make(map[string]*loginBucket), now: time.Now}
}

// allow records an attempt for key and reports whether it is still within
// the window's budget.
func (l *loginLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	// Opportunistic pruning keeps the map bounded without a background
	// goroutine; only matters if someone sprays usernames/IPs.
	if len(l.buckets) > 4096 {
		for k, b := range l.buckets {
			if now.After(b.resetAt) {
				delete(l.buckets, k)
			}
		}
	}

	b, ok := l.buckets[key]
	if !ok || now.After(b.resetAt) {
		b = &loginBucket{resetAt: now.Add(loginWindow)}
		l.buckets[key] = b
	}
	b.count++
	return b.count <= maxLoginAttempts
}

// reset clears a key's counter — called on successful login so a user who
// eventually gets their password right isn't locked out next session.
func (l *loginLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.buckets, key)
}

// clientIP extracts the address middleware.RealIP resolved, without the
// ephemeral port so one client maps to one bucket.
func clientIP(remoteAddr string) string {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	return remoteAddr
}
