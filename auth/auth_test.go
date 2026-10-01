package auth

import (
	"net/http/httptest"
	"testing"
	"time"

	"liapi/common"
	"liapi/config"
)

func TestDeviceHashAuth(t *testing.T) {
	raw := common.RandomToken("sk-")
	hash := common.HashToken(raw)
	cfg := &config.Config{
		ClientTokens: []string{"legacy-token"},
		Devices: []config.Device{
			{ID: "d1", TokenHash: hash},
			{ID: "off", TokenHash: common.HashToken("sk-off"), Disabled: true},
		},
	}
	cfg.SetDefaults()
	cfg.ClientTokens = []string{"legacy-token"}
	cfg.Devices = []config.Device{
		{ID: "d1", TokenHash: hash},
		{ID: "off", TokenHash: common.HashToken("sk-off"), Disabled: true},
	}
	h := config.NewHolder(cfg)
	c := NewClient(h)

	// Device token authenticates via hash; plaintext is never stored.
	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	r.Header.Set("Authorization", "Bearer "+raw)
	id := c.Identify(r)
	if id == nil {
		t.Fatal("device token should authenticate")
	}
	if id.DeviceID != "d1" || id.Device == nil {
		t.Fatalf("identity missing device: %+v", id)
	}
	if id.Legacy {
		t.Fatal("device auth must not be flagged legacy")
	}
	// Wrong password rejected.
	r.Header.Set("Authorization", "Bearer "+raw+"x")
	if c.Identify(r) != nil {
		t.Fatal("tampered token must fail")
	}
	// Disabled device rejected.
	r.Header.Set("Authorization", "Bearer sk-off")
	if c.Identify(r) != nil {
		t.Fatal("disabled device must fail")
	}
	// Legacy plaintext token still works.
	r.Header.Set("Authorization", "Bearer legacy-token")
	id = c.Identify(r)
	if id == nil || !id.Legacy {
		t.Fatalf("legacy token should auth: %+v", id)
	}
	// x-api-key and ?token sources.
	r2 := httptest.NewRequest("POST", "/v1/x?token="+raw, nil)
	if c.Identify(r2) == nil {
		t.Fatal("?token= source should work")
	}
	r3 := httptest.NewRequest("POST", "/v1/x", nil)
	r3.Header.Set("x-api-key", raw)
	if c.Identify(r3) == nil {
		t.Fatal("x-api-key source should work")
	}
}

func TestBearerParse(t *testing.T) {
	if got := bearerFromHeader("Bearer abc"); got != "abc" {
		t.Fatalf("got %q", got)
	}
	if got := bearerFromHeader("bearer abc"); got != "abc" {
		t.Fatalf("case-insensitive prefix failed: %q", got)
	}
	if got := bearerFromHeader("Basic abc"); got != "" {
		t.Fatalf("non-bearer scheme: %q", got)
	}
	if got := bearerFromHeader("Bearer"); got != "" {
		t.Fatalf("missing value: %q", got)
	}
}

func TestHashEqual(t *testing.T) {
	h := common.HashToken("secret")
	if !common.HashEqual("secret", h) {
		t.Fatal("matching hash")
	}
	if common.HashEqual("secret2", h) {
		t.Fatal("mismatching hash")
	}
}

func TestAdminLoginAndSession(t *testing.T) {
	hash, err := common.HashPassword("hunter2")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		AdminUsername:     "alice",
		AdminPasswordHash: hash,
	}
	cfg.SetDefaults()
	cfg.AdminUsername = "alice"
	cfg.AdminPasswordHash = hash
	h := config.NewHolder(cfg)
	a := NewAdmin(h)

	// Wrong password / username rejected.
	if _, ok := a.Login("alice", "nope"); ok {
		t.Fatal("wrong password must fail")
	}
	if _, ok := a.Login("bob", "hunter2"); ok {
		t.Fatal("wrong username must fail")
	}

	// Correct credentials issue a session token that authenticates.
	tok, ok := a.Login("alice", "hunter2")
	if !ok || tok == "" {
		t.Fatal("valid login should issue a token")
	}
	r := httptest.NewRequest("GET", "/admin/api/overview", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	if !a.Authenticate(r) {
		t.Fatal("session token should authenticate")
	}

	// Anonymous access is refused while login is enabled.
	r0 := httptest.NewRequest("GET", "/admin/api/overview", nil)
	if a.Authenticate(r0) {
		t.Fatal("anonymous request must not authenticate once login is enabled")
	}

	// Unknown/garbage token rejected.
	r3 := httptest.NewRequest("GET", "/x", nil)
	r3.Header.Set("Authorization", "Bearer sess-garbage")
	if a.Authenticate(r3) {
		t.Fatal("unknown session token must fail")
	}

	// Logout invalidates the session.
	a.Logout(tok)
	if a.Authenticate(r) {
		t.Fatal("logged-out session must fail")
	}
}

// TestOpenConsoleWhenLoginDisabled: with no username/password configured the
// admin surface is open — anonymous requests authenticate, which is what makes
// the console reachable without any token.
func TestOpenConsoleWhenLoginDisabled(t *testing.T) {
	cfg := &config.Config{}
	cfg.SetDefaults()
	a := NewAdmin(config.NewHolder(cfg))

	for _, target := range []string{"/admin/api/overview", "/metrics"} {
		r := httptest.NewRequest("GET", target, nil)
		if !a.Authenticate(r) {
			t.Fatalf("%s must be reachable while login is disabled", target)
		}
	}
	// A garbage credential is still meaningless, but it does not lock anyone
	// out: the open gate ignores credentials entirely.
	r := httptest.NewRequest("GET", "/admin/api/overview", nil)
	r.Header.Set("Authorization", "Bearer sess-garbage")
	if !a.Authenticate(r) {
		t.Fatal("open console must not reject requests carrying junk credentials")
	}
}

func TestAdminSessionExpiry(t *testing.T) {
	hash, _ := common.HashPassword("pw")
	cfg := &config.Config{AdminUsername: "u", AdminPasswordHash: hash}
	cfg.SetDefaults()
	cfg.AdminUsername = "u"
	cfg.AdminPasswordHash = hash
	a := NewAdmin(config.NewHolder(cfg))
	a.SetSessionTTL(time.Nanosecond)
	tok, ok := a.Login("u", "pw")
	if !ok {
		t.Fatal("login should succeed")
	}
	time.Sleep(2 * time.Millisecond)
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	if a.Authenticate(r) {
		t.Fatal("expired session must fail")
	}
}
