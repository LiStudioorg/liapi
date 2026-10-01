package auth

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"liapi/common"
	"liapi/config"
)

const (
	defaultLockoutFailures = 5
	defaultLockoutWindow   = 5 * time.Minute
	defaultLockoutDuration = 15 * time.Minute

	// defaultSessionTTL bounds how long a login session stays valid.
	defaultSessionTTL = 12 * time.Hour
)

type lockState struct {
	fails       int
	windowStart time.Time
	lockedUntil time.Time
}

type session struct {
	username  string
	expiresAt time.Time
}

// Admin validates access to /admin/api/* and the admin UI.
//
// Two credentials are accepted:
//   - a browser login session token (issued by Login, short-lived), and
//   - the static admin_token from config (kept for /metrics and scripts).
type Admin struct {
	holder *config.Holder

	mu       sync.Mutex
	locks    map[string]*lockState
	sessions map[string]*session
	ttl      time.Duration
}

func NewAdmin(holder *config.Holder) *Admin {
	return &Admin{
		holder:   holder,
		locks:    make(map[string]*lockState),
		sessions: make(map[string]*session),
		ttl:      defaultSessionTTL,
	}
}

// SetSessionTTL overrides the default session lifetime (tests, tuning).
func (a *Admin) SetSessionTTL(d time.Duration) {
	if d > 0 {
		a.ttl = d
	}
}

// LoginEnabled reports whether username/password login is currently on
// (explicit login_enabled switch, or "a password is set" when unset).
func (a *Admin) LoginEnabled() bool {
	return a.holder.Get().LoginAllowed()
}

// Login validates username/password against the configured admin credentials
// and, on success, returns a fresh session token. The second result is false
// when the credentials are wrong or password login is not configured/enabled.
func (a *Admin) Login(username, password string) (string, bool) {
	cfg := a.holder.Get()
	if !cfg.LoginAllowed() || cfg.AdminPasswordHash == "" {
		return "", false
	}
	if !common.TokenEqual(cfg.AdminUsername, strings.TrimSpace(username)) {
		return "", false
	}
	if !common.VerifyPassword(cfg.AdminPasswordHash, password) {
		return "", false
	}
	tok := common.RandomToken("sess-")
	now := time.Now()
	a.mu.Lock()
	// Opportunistically drop expired sessions so the map stays bounded.
	for k, s := range a.sessions {
		if now.After(s.expiresAt) {
			delete(a.sessions, k)
		}
	}
	a.sessions[tok] = &session{username: cfg.AdminUsername, expiresAt: now.Add(a.ttl)}
	a.mu.Unlock()
	return tok, true
}

// Logout invalidates a session token (no-op if unknown).
func (a *Admin) Logout(token string) {
	a.mu.Lock()
	delete(a.sessions, token)
	a.mu.Unlock()
}

// sessionValid reports whether token is a live login session.
func (a *Admin) sessionValid(token string) bool {
	if token == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.sessions[token]
	if s == nil {
		return false
	}
	if time.Now().After(s.expiresAt) {
		delete(a.sessions, token)
		return false
	}
	return true
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

// Credential extracts the admin credential from the request, checking:
//  1. Authorization: Bearer <session-token>
//  2. ?token=<session-token>   (handy for scrapers/bookmarks)
func (a *Admin) Credential(r *http.Request) string {
	if t := bearerFromHeader(r.Header.Get("Authorization")); t != "" {
		return t
	}
	return strings.TrimSpace(r.URL.Query().Get("token"))
}

// Authenticate reports whether the request is allowed to reach the admin API.
//
// When username/password login is switched off (the default on a fresh
// install) the console is open: whoever can reach the port is the operator,
// and the IP allowlist is the only gate. Once login_enabled=true, a live
// login session is the only accepted credential.
func (a *Admin) Authenticate(r *http.Request) bool {
	if !a.holder.Get().LoginAllowed() {
		return true
	}
	t := a.Credential(r)
	if t == "" {
		return false
	}
	return a.sessionValid(t)
}
