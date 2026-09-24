package server

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"liapi/config"
)

// TestTokenPolicyExpiry rejects requests with an expired token policy.
func TestTokenPolicyExpiry(t *testing.T) {
	srv, _ := newTestServer(t, func(c *config.Config) {
		c.TokenPolicies = map[string]config.TokenPolicy{
			"sk-client-0123456789": {ExpiresAt: time.Now().Add(-time.Hour).Format(time.RFC3339)},
		}
	})
	rec := do(srv.Handler(), "GET", "/v1/models", "sk-client-0123456789", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expired token: want 401, got %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "token_expired") {
		t.Fatalf("error code should be token_expired: %s", rec.Body.String())
	}
}

// TestTokenPolicyIPAllowlist blocks non-allowlisted source IPs.
func TestTokenPolicyIPAllowlist(t *testing.T) {
	srv, _ := newTestServer(t, func(c *config.Config) {
		c.TokenPolicies = map[string]config.TokenPolicy{
			"sk-client-0123456789": {AllowIPs: []string{"203.0.113.9"}},
		}
	})
	// httptest RemoteAddr is 192.0.2.1:1234 — not in allowlist.
	rec := do(srv.Handler(), "GET", "/v1/models", "sk-client-0123456789", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 for disallowed IP, got %d", rec.Code)
	}
	// Spoofed XFF is honored only when... clientIP always honors XFF currently.
	req := newRequest("GET", "/v1/models", "sk-client-0123456789", "")
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	rec2 := serve(srv, req)
	if rec2.Code != http.StatusOK {
		t.Fatalf("allowlisted IP should pass, got %d %s", rec2.Code, rec2.Body.String())
	}
}

// TestTokenPolicyRPM enforces a per-token rate limit tighter than global.
func TestTokenPolicyRPM(t *testing.T) {
	srv, _ := newTestServer(t, func(c *config.Config) {
		c.RateLimitPerMinute = 0 // global unlimited
		c.TokenPolicies = map[string]config.TokenPolicy{
			"sk-client-0123456789": {RPM: 2},
		}
	})
	h := srv.Handler()
	ok, limited := 0, 0
	for i := 0; i < 5; i++ {
		rec := do(h, "GET", "/v1/models", "sk-client-0123456789", "")
		if rec.Code == http.StatusTooManyRequests {
			limited++
		} else {
			ok++
		}
	}
	if limited == 0 || ok == 0 {
		t.Fatalf("expected some pass + some 429, got ok=%d limited=%d", ok, limited)
	}
}

// TestAdminLockout locks an IP after repeated bad admin tokens.
func TestAdminLockout(t *testing.T) {
	srv, _ := newTestServer(t, nil)
	h := srv.Handler()
	// 5 bad attempts trigger lockout; 6th (even with good token) is locked.
	for i := 0; i < 6; i++ {
		do(h, "GET", "/admin/api/overview", "wrong-token", "")
	}
	rec := do(h, "GET", "/admin/api/overview", adminTok, "")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429 locked out after failures, got %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "admin_locked") {
		t.Fatalf("lockout error code expected: %s", rec.Body.String())
	}
}

// TestAdminAuditRecordsEvents exposes the audit ring via the admin API.
func TestAdminAuditRecordsEvents(t *testing.T) {
	srv, _ := newTestServer(t, nil)
	h := srv.Handler()
	// One failed + one successful attempt.
	do(h, "GET", "/admin/api/overview", "bad", "")
	do(h, "GET", "/admin/api/overview", adminTok, "")
	rec := do(h, "GET", "/admin/api/audit?n=10", adminTok, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("audit: %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"ok":false`) || !strings.Contains(body, `"ok":true`) {
		t.Fatalf("audit should contain both failed and successful events: %s", body)
	}
}

// TestProbeNowAndHistory runs the manual probe endpoint against a fake
// upstream and checks the history ring picks it up.
func TestProbeNowAndHistory(t *testing.T) {
	up := newOKUpstream(t)
	srv, _ := newTestServer(t, func(c *config.Config) {
		c.Upstreams = []config.Upstream{{
			Name: "u1", BaseURL: up.URL, APIKey: "k", Models: []string{"*"},
		}}
	})
	h := srv.Handler()
	rec := do(h, "POST", "/admin/api/health/probe", adminTok, `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("probe: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"u1"`) {
		t.Fatalf("probe result should include u1: %s", rec.Body.String())
	}
	rec = do(h, "GET", "/admin/api/health/history?n=10", adminTok, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("history: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"manual":true`) {
		t.Fatalf("manual probe should be marked in history: %s", rec.Body.String())
	}
}

// TestMultimodalCounted verifies image parts flow into the log/stats.
func TestMultimodalCounted(t *testing.T) {
	up := newOKUpstream(t)
	srv, _ := newTestServer(t, func(c *config.Config) {
		c.Upstreams = []config.Upstream{{
			Name: "u1", BaseURL: up.URL, APIKey: "k", Models: []string{"m"},
		}}
	})
	h := srv.Handler()
	body := `{"model":"m","messages":[{"role":"user","content":[
		{"type":"text","text":"what is this"},
		{"type":"image_url","image_url":{"url":"https://x/y.png"}}
	]}]}`
	rec := do(h, "POST", "/v1/chat/completions", "sk-client-0123456789", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	// Poll async log ring for the multimodal count.
	for i := 0; i < 100; i++ {
		for _, e := range srv.logger.Recent(5) {
			if e.Multimodal == 1 {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected multimodal=1 in log, got %+v", srv.logger.Recent(5))
}

// TestRouteGroupHeader narrows candidates by upstream.group.
func TestRouteGroupHeader(t *testing.T) {
	var hit string
	free := newOKUpstreamNamed(t, "free-up", &hit)
	paid := newOKUpstreamNamed(t, "paid-up", &hit)
	srv, _ := newTestServer(t, func(c *config.Config) {
		c.Upstreams = []config.Upstream{
			{Name: "free", BaseURL: free.URL, APIKey: "k", Models: []string{"m"}, Group: "free"},
			{Name: "paid", BaseURL: paid.URL, APIKey: "k", Models: []string{"m"}, Group: "paid"},
		}
	})
	req := newRequest("POST", "/v1/chat/completions", "sk-client-0123456789",
		`{"model":"m","messages":[]}`)
	req.Header.Set("X-Route-Group", "paid")
	rec := serve(srv, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if hit != "paid-up" {
		t.Fatalf("group header should route to paid upstream, hit=%q", hit)
	}
}
