package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"liapi/auth"
	"liapi/common"
	"liapi/config"
	"liapi/relay"
	"liapi/routing"
	"liapi/stats"
)

// newTestServer wires a full Server against a temp config file.
func newTestServer(t *testing.T, mutate func(*config.Config)) (*Server, *config.Holder) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &config.Config{
		ClientTokens:       []string{"sk-client-0123456789"},
		RateLimitPerMinute: 0,
		HealthStateFile:    filepath.Join(dir, "health_state.json"),
		LogFile:            filepath.Join(dir, "relay.jsonl"),
	}
	cfg.SetDefaults()
	cfg.ClientTokens = []string{"sk-client-0123456789"}
	cfg.Devices = nil
	if mutate != nil {
		mutate(cfg)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("mutate produced invalid config: %v", err)
	}
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(filepath.Join(dir, "health_state.json"))

	holder := config.NewHolder(loaded)
	logger, err := stats.NewLogger(filepath.Join(dir, "relay.jsonl"), 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = logger.Close() })

	limiter := stats.NewLimiter(loaded.RateLimitPerMinute)
	t.Cleanup(limiter.Close)
	totals := stats.NewTotals()
	rel := relay.New(holder)
	health := stats.NewHealth(holder, rel.Client())
	router := routing.NewRouter(holder, health)
	clientAuth := auth.NewClient(holder)
	adminAuth := auth.NewAdmin(holder)
	srv := New(holder, router, rel, clientAuth, adminAuth, logger, limiter, health, totals, path)
	return srv, holder
}

func do(handler http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

const adminTok = "adm-secret-0123456789"

func TestRequestIDEchoAndGenerate(t *testing.T) {
	srv, _ := newTestServer(t, nil)
	h := srv.Handler()

	// Generated when absent.
	rec := do(h, "GET", "/admin/api/overview", adminTok, "")
	if got := rec.Header().Get("X-Request-ID"); got == "" || len(got) < 8 {
		t.Fatalf("expected generated request id, got %q", got)
	}
	// Client-provided id is echoed.
	req := httptest.NewRequest("GET", "/admin/api/overview", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	req.Header.Set("X-Request-ID", "trace-abc-123")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("X-Request-ID"); got != "trace-abc-123" {
		t.Fatalf("client request id not echoed: %q", got)
	}
}

func TestAdminRateLimitPerIP(t *testing.T) {
	srv, _ := newTestServer(t, func(c *config.Config) {
		c.AdminRatePerMinute = 3
	})
	h := srv.Handler()
	// All requests come from the same RemoteAddr (httptest default).
	denied := 0
	for i := 0; i < 6; i++ {
		rec := do(h, "GET", "/admin/api/overview", adminTok, "")
		if rec.Code == http.StatusTooManyRequests {
			denied++
		}
	}
	if denied == 0 {
		t.Fatal("expected 429 after exhausting admin rate limit")
	}
}

func TestAdminIPAllowlist(t *testing.T) {
	srv, _ := newTestServer(t, func(c *config.Config) {
		c.AdminAllowIPs = []string{"203.0.113.7"}
	})
	h := srv.Handler()
	// httptest RemoteAddr is 192.0.2.1:1234 — not in the allowlist.
	rec := do(h, "GET", "/admin/api/overview", adminTok, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 for IP not in allowlist, got %d", rec.Code)
	}
	// A forged X-Forwarded-For from a stranger must NOT bypass the allowlist:
	// the header is honored only from loopback or allowlisted proxy peers.
	req := httptest.NewRequest("GET", "/admin/api/overview", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("forged XFF from stranger must stay 403, got %d (%s)", rec2.Code, rec2.Body.String())
	}
}

func TestClientIPProxyTrust(t *testing.T) {
	mk := func(remote, xff string) *http.Request {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = remote
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		return r
	}
	allow := []string{"198.51.100.0/24"}
	cases := []struct {
		name, remote, xff, allow0, want string
	}{
		{"no header", "203.0.113.5:4321", "", "", "203.0.113.5"},
		{"stranger cannot forge", "203.0.113.5:4321", "8.8.8.8", "", "203.0.113.5"},
		{"stranger cannot forge even with allowlist", "203.0.113.5:4321", "8.8.8.8", "allow", "203.0.113.5"},
		{"loopback proxy trusted", "127.0.0.1:5555", "8.8.8.8, 10.0.0.1", "", "8.8.8.8"},
		{"loopback v6 proxy trusted", "[::1]:5555", "8.8.8.8", "", "8.8.8.8"},
		{"allowlisted proxy trusted", "198.51.100.7:5555", "8.8.8.8", "allow", "8.8.8.8"},
	}
	for _, c := range cases {
		var al []string
		if c.allow0 == "allow" {
			al = allow
		}
		if got := clientIP(mk(c.remote, c.xff), al); got != c.want {
			t.Errorf("%s: clientIP = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDeviceLifecycleAndAuth(t *testing.T) {
	srv, holder := newTestServer(t, nil)
	h := srv.Handler()

	// Create device → plaintext returned once, only hash stored.
	rec := do(h, "POST", "/admin/api/devices", adminTok,
		`{"id":"dev1","name":"phone","rpm":10}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("create device: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		OK     bool           `json:"ok"`
		Token  string         `json:"token"`
		Device map[string]any `json:"device"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Token == "" || !strings.HasPrefix(created.Token, "sk-") {
		t.Fatalf("plaintext token missing: %+v", created)
	}
	// Config must NOT contain the plaintext token.
	stored, _ := json.Marshal(holder.Get())
	if strings.Contains(string(stored), created.Token) {
		t.Fatal("plaintext device token must never be persisted")
	}
	// Device usable for client auth.
	rec = do(h, "GET", "/v1/models", created.Token, "")
	if rec.Code != http.StatusOK && rec.Code != http.StatusBadGateway {
		// No upstreams configured → models list still OK (empty set).
		t.Fatalf("device token should auth on /v1/models, got %d %s", rec.Code, rec.Body.String())
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("models: %d %s", rec.Code, rec.Body.String())
	}

	// Rotate: old token dies, new token works.
	rec = do(h, "POST", "/admin/api/devices/dev1/rotate", adminTok, `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("rotate: %d %s", rec.Code, rec.Body.String())
	}
	var rot struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &rot); err != nil {
		t.Fatal(err)
	}
	if rot.Token == "" || rot.Token == created.Token {
		t.Fatal("rotate should issue a fresh token")
	}
	rec = do(h, "GET", "/v1/models", created.Token, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("old token must be rejected after rotate, got %d", rec.Code)
	}
	rec = do(h, "GET", "/v1/models", rot.Token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("new token must work, got %d %s", rec.Code, rec.Body.String())
	}

	// Delete device → rejected.
	rec = do(h, "DELETE", "/admin/api/devices/dev1", adminTok, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d", rec.Code)
	}
	rec = do(h, "GET", "/v1/models", rot.Token, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("deleted device token must fail, got %d", rec.Code)
	}
}

func TestMetricsAuth(t *testing.T) {
	// No metrics_token → the metrics endpoint follows the console gate, which
	// is open while login is disabled.
	srv, _ := newTestServer(t, nil)
	h := srv.Handler()
	rec := do(h, "GET", "/metrics", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("metrics on an open console: want 200, got %d", rec.Code)
	}
	// Once login is enabled the gate closes and /metrics needs a session.
	hash, err := common.HashPassword("sup3r-secret")
	if err != nil {
		t.Fatal(err)
	}
	gatedSrv, _ := newTestServer(t, func(c *config.Config) {
		c.AdminUsername = "root"
		c.AdminPasswordHash = hash
	})
	if rec := do(gatedSrv.Handler(), "GET", "/metrics", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("metrics without credential while login enabled: want 401, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "liapi_requests_total") {
		t.Fatalf("metrics body missing counters: %s", rec.Body.String())
	}

	// Dedicated metrics token.
	srv2, _ := newTestServer(t, func(c *config.Config) { c.MetricsToken = "m-secret" })
	h2 := srv2.Handler()
	rec = do(h2, "GET", "/metrics?token=m-secret", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("metrics with metrics_token: got %d", rec.Code)
	}
	rec = do(h2, "GET", "/metrics", "not-the-metrics-token", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("metrics_token set: a wrong credential must not pass, got %d", rec.Code)
	}

	// Open mode "-".
	srv3, _ := newTestServer(t, func(c *config.Config) { c.MetricsToken = "-" })
	rec = do(srv3.Handler(), "GET", "/metrics", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("metrics_token=- should be open, got %d", rec.Code)
	}
}

func TestConfigMaskedRoundtripKeepsSecrets(t *testing.T) {
	srv, holder := newTestServer(t, func(c *config.Config) {
		c.Upstreams = []config.Upstream{{
			Name: "up1", BaseURL: "https://api.openai.com/v1", APIKey: "sk-upstream-real-key",
			Models: []string{"gpt-4o"},
		}}
	})
	h := srv.Handler()

	// GET returns masked config.
	rec := do(h, "GET", "/admin/api/config", adminTok, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get config: %d", rec.Code)
	}
	var masked map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &masked); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rec.Body.String(), "sk-upstream-real-key") {
		t.Fatal("GET config must mask api keys")
	}

	// POST the masked config back — secrets must be restored on disk.
	rec = do(h, "POST", "/admin/api/config", adminTok, rec.Body.String())
	if rec.Code != http.StatusOK {
		t.Fatalf("replace config with masked payload: %d %s", rec.Code, rec.Body.String())
	}
	live := holder.Get()
	if live.Upstreams[0].APIKey != "sk-upstream-real-key" {
		t.Fatalf("masked api_key was not restored: %q", live.Upstreams[0].APIKey)
	}
	if strings.Contains(rec.Body.String(), `"admin_token"`) {
		t.Fatal("GET config must not expose an admin token field")
	}
	// File on disk still has the real key.
	data, err := os.ReadFile(srv.configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "sk-upstream-real-key") {
		t.Fatal("real key must be persisted after masked roundtrip")
	}
}

func TestConfigImportValidationKeepsOldOnFailure(t *testing.T) {
	srv, holder := newTestServer(t, nil)
	h := srv.Handler()
	before := holder.Get()

	// Invalid config → 400, holder unchanged (atomic swap).
	rec := do(h, "POST", "/admin/api/config/import", adminTok,
		`{"upstreams":[{"name":"x","base_url":"nope"}]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid import want 400, got %d", rec.Code)
	}
	if holder.Get() != before {
		t.Fatal("failed import must not swap config")
	}
}

func TestOneAPIImportEndpoint(t *testing.T) {
	srv, holder := newTestServer(t, nil)
	h := srv.Handler()
	body := `{"channels":[{"id":1,"name":"oa","type":1,"key":"sk-oa","models":["gpt-4o"],"base_url":"","status":1}]}`
	rec := do(h, "POST", "/admin/api/config/import?format=oneapi", adminTok, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("oneapi import: %d %s", rec.Code, rec.Body.String())
	}
	if len(holder.Get().Upstreams) != 1 || holder.Get().Upstreams[0].BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("oneapi upstreams not applied: %+v", holder.Get().Upstreams)
	}
	// Non-admin rejected — only meaningful once login is enabled.
	hash, err := common.HashPassword("sup3r-secret")
	if err != nil {
		t.Fatal(err)
	}
	srv2, _ := newTestServer(t, func(c *config.Config) {
		c.AdminUsername = "root"
		c.AdminPasswordHash = hash
	})
	rec = do(srv2.Handler(), "POST", "/admin/api/config/import?format=oneapi", "wrong", body)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 without a session while login is enabled, got %d", rec.Code)
	}
}

func TestStatsEndpoints(t *testing.T) {
	srv, _ := newTestServer(t, nil)
	// Feed the aggregate directly (handler path needs a live upstream).
	srv.Aggregate().Add(stats.Entry{
		Time: "2026-09-23T10:00:00Z", Model: "m1", Device: "d1",
		Token: "sk-a...bcd", Status: 200, LatencyMS: 100, InTokens: 3, OutTokens: 4, Cost: 0.5,
	})
	h := srv.Handler()
	rec := do(h, "GET", "/admin/api/stats?group_by=model", adminTok, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("stats: %d", rec.Code)
	}
	var snap stats.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if len(snap.Rows) != 1 || snap.Rows[0].Requests != 1 {
		t.Fatalf("stats rows: %+v", snap.Rows)
	}
	if snap.Rows[0].P50MS == 0 {
		t.Fatal("percentiles missing")
	}

	rec = do(h, "GET", "/admin/api/stats/export?format=csv", adminTok, "")
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Body.String(), "key,") {
		t.Fatalf("csv export: %d %q", rec.Code, rec.Body.String())
	}
	rec = do(h, "GET", "/admin/api/stats/devices?n=5", adminTok, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "d1") {
		t.Fatalf("device ranking: %d %s", rec.Code, rec.Body.String())
	}
}

func TestConfigExportEndpoint(t *testing.T) {
	srv, _ := newTestServer(t, func(c *config.Config) {
		c.Upstreams = []config.Upstream{{
			Name: "up1", BaseURL: "https://api.openai.com/v1", APIKey: "sk-upstream-real-key",
			Models: []string{"gpt-4o"},
		}}
	})
	h := srv.Handler()
	rec := do(h, "GET", "/admin/api/config/export", adminTok, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("export: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "sk-upstream-real-key") {
		t.Fatal("export must include real upstream keys (admin-gated backup)")
	}
	// Roundtrip the exported bytes through import.
	rec2 := do(h, "POST", "/admin/api/config/import", adminTok, rec.Body.String())
	if rec2.Code != http.StatusOK {
		t.Fatalf("reimport: %d %s", rec2.Code, rec2.Body.String())
	}
}

func TestAdminLoginFlow(t *testing.T) {
	const username = "root"
	const password = "sup3r-secret"
	srv, holder := newTestServer(t, nil)
	// Seed a known password on the live config.
	hash, err := common.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	cur := holder.Get().Clone()
	cur.AdminUsername = username
	cur.AdminPasswordHash = hash
	holder.Set(cur)
	h := srv.Handler()

	// Wrong password → 401.
	rec := do(h, "POST", "/admin/api/login", "", `{"username":"root","password":"nope"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad login want 401, got %d", rec.Code)
	}

	// Correct password → session token.
	rec = do(h, "POST", "/admin/api/login", "", `{"username":"root","password":"sup3r-secret"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("login want 200, got %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Token == "" {
		t.Fatalf("login response: %s", rec.Body.String())
	}

	// Session token authorizes admin API.
	rec = do(h, "GET", "/admin/api/me", out.Token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("session auth want 200, got %d", rec.Code)
	}

	// Logout invalidates it.
	rec = do(h, "POST", "/admin/api/logout", out.Token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("logout want 200, got %d", rec.Code)
	}
	rec = do(h, "GET", "/admin/api/me", out.Token, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("post-logout want 401, got %d", rec.Code)
	}

	// While login is enabled, anonymous access stays refused.
	rec = do(h, "GET", "/admin/api/me", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous while login enabled: want 401, got %d", rec.Code)
	}
}

// Fresh installs ship with no credentials: login is disabled, the login
// endpoint refuses with 401/403, but the static admin token still works.
func TestAdminLoginDisabled(t *testing.T) {
	srv, _ := newTestServer(t, nil) // no password set by default anymore
	h := srv.Handler()

	// login-info advertises the disabled state.
	rec := do(h, "GET", "/admin/api/login-info", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login-info want 200, got %d", rec.Code)
	}
	var info struct {
		LoginEnabled bool `json:"login_enabled"`
		PasswordSet  bool `json:"password_set"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info.LoginEnabled || info.PasswordSet {
		t.Fatalf("fresh config should have login off: %+v", info)
	}

	// Password login refuses even with plausible credentials.
	rec = do(h, "POST", "/admin/api/login", "", `{"username":"admin","password":"x"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("login while disabled want 403, got %d", rec.Code)
	}

	// The console is open on a fresh install: no credential needed.
	rec = do(h, "GET", "/admin/api/me", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("open console want 200, got %d", rec.Code)
	}

	// Setting a plaintext admin_password through the config API hashes it and
	// turns login on; the plaintext never appears in GET /config.
	rec = do(h, "POST", "/admin/api/config", "",
		`{"admin_username":"myuser","admin_password":"hunter2-proper","login_enabled":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("set password want 200, got %d %s", rec.Code, rec.Body.String())
	}

	// login-info now reports an enabled login with a password set.
	rec = do(h, "GET", "/admin/api/login-info", "", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if !info.LoginEnabled || !info.PasswordSet {
		t.Fatalf("after setting password: %+v", info)
	}

	// The configured password logs in.
	rec = do(h, "POST", "/admin/api/login", "", `{"username":"myuser","password":"hunter2-proper"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("login after opt-in want 200, got %d %s", rec.Code, rec.Body.String())
	}
	var sess struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &sess); err != nil || sess.Token == "" {
		t.Fatalf("login response: %s", rec.Body.String())
	}

	// GET /config never leaks the plaintext or the raw hash. Login is on now,
	// so this needs the session.
	rec = do(h, "GET", "/admin/api/config", sess.Token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get config want 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "hunter2-proper") {
		t.Fatal("plaintext password leaked in GET /config")
	}
	if strings.Contains(body, "pbkdf2-sha256$") {
		t.Fatal("raw password hash leaked in GET /config")
	}
	if !strings.Contains(body, `"password_set":true`) {
		t.Fatalf("password_set hint missing: %s", body)
	}
}
