package auth

import (
	"net/http"
	"sync"
	"time"

	"liapi/common"
	"liapi/config"
)

const (
	defaultLockoutFailures = 5
	defaultLockoutWindow   = 5 * time.Minute
	defaultLockoutDuration = 15 * time.Minute
)

type lockState struct {
	fails       int
	windowStart time.Time
	lockedUntil time.Time
}

// Admin validates the separate admin token used for /admin/api/*, with
// per-IP lockout after repeated failures (brute-force protection).
type Admin struct {
	holder *config.Holder

	mu    sync.Mutex
	locks map[string]*lockState
}

func NewAdmin(holder *config.Holder) *Admin {
	return &Admin{holder: holder, locks: make(map[string]*lockState)}
}

// Locked reports whether ip is currently locked out (and for how long).
func (a *Admin) Locked(ip string) (bool, time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.locks[ip]
	if s == nil {
		return false, 0
	}
	if d := time.Until(s.lockedUntil); d > 0 {
		return true, d
	}
	return false, 0
}

// RecordFailure increments the failure counter for ip and locks the IP
// when the threshold is crossed.
func (a *Admin) RecordFailure(ip string) (lockedNow bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	s := a.locks[ip]
	if s == nil {
		s = &lockState{windowStart: now}
		a.locks[ip] = s
	}
	if now.Sub(s.windowStart) > defaultLockoutWindow {
		s.windowStart = now
		s.fails = 0
	}
	s.fails++
	if s.fails >= defaultLockoutFailures {
		s.lockedUntil = now.Add(defaultLockoutDuration)
		return true
	}
	return false
}

// ClearFailures resets the counter after a successful authentication.
func (a *Admin) ClearFailures(ip string) {
	a.mu.Lock()
	delete(a.locks, ip)
	a.mu.Unlock()
}

func (a *Admin) Authenticate(r *http.Request) bool {
	cfg := a.holder.Get()
	if cfg.AdminToken == "" {
		return false
	}
	t := bearerFromHeader(r.Header.Get("Authorization"))
	if t == "" {
		t = r.Header.Get("x-admin-token")
	}
	if t == "" {
		return false
	}
	return common.TokenEqual(cfg.AdminToken, t)
}
