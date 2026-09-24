package relay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"liapi/config"
)

func testRelay(cfg *config.Config) (*Relay, *config.Holder) {
	cfg.SetDefaults()
	if cfg.AdminToken == "" {
		cfg.AdminToken = "adm-x"
	}
	h := config.NewHolder(cfg)
	return New(h), h
}

// TestRetryOnNetworkError: connection-refused style failures always retry the
// next attempt/upstream (safe: request never reached a processing upstream).
func TestFailoverOnNetworkError(t *testing.T) {
	var hitsA atomic.Int32
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsA.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer a.Close()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer b.Close()

	cfg := &config.Config{
		Upstreams: []config.Upstream{
			{Name: "a", BaseURL: a.URL, APIKey: "k", Models: []string{"m"}, Retry: 1},
			{Name: "b", BaseURL: b.URL, APIKey: "k", Models: []string{"m"}},
		},
	}
	rl, _ := testRelay(cfg)
	// Close A so subsequent requests fail at the network layer? A is up but
	// returns 500 — that exercises failUpstream. For pure network error, point
	// at a dead port.
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close() // now connection refused

	cfg.Upstreams[0].BaseURL = deadURL
	cands := []config.Upstream{cfg.Upstreams[0], cfg.Upstreams[1]}
	var events []string
	rl.SetObserver(func(kind, from, to string) {
		events = append(events, kind+":"+from+"->"+to)
	})
	res, f := rl.Do(context.Background(), []byte(`{"model":"m"}`), "/v1/chat/completions", "m", "m", cands, nil, "rid-1")
	if f != nil {
		t.Fatalf("expected success via failover, got %v", f)
	}
	defer res.Close()
	if res.Upstream != "b" {
		t.Fatalf("should have failed over to b, got %s", res.Upstream)
	}
	if len(events) == 0 {
		t.Fatal("failover event not emitted")
	}
}

// TestNoRetryOnTimeout: with retry_on_timeout=false (default), a timeout must
// NOT be retried nor failed over (POST may already have been processed).
func TestNoRetryOnTimeoutByDefault(t *testing.T) {
	var aHits, bHits atomic.Int32
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		aHits.Add(1)
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer a.Close()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bHits.Add(1)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer b.Close()

	cfg := &config.Config{Timeout: config.Duration{Duration: 50 * time.Millisecond}}
	rl, _ := testRelay(cfg)
	cands := []config.Upstream{
		{Name: "a", BaseURL: a.URL, APIKey: "k", Models: []string{"m"}, Retry: 2},
		{Name: "b", BaseURL: b.URL, APIKey: "k", Models: []string{"m"}},
	}
	res, f := rl.Do(context.Background(), []byte(`{"model":"m"}`), "/v1/chat/completions", "m", "m", cands, nil, "rid-2")
	if f == nil {
		res.Close()
		t.Fatal("timeout must fail when retry_on_timeout=false")
	}
	st, _ := ErrStatus(f)
	if st != http.StatusGatewayTimeout {
		t.Fatalf("timeout should map to 504, got %d (%s)", st, f)
	}
	if bHits.Load() != 0 {
		t.Fatalf("must not failover on timeout (b hit %d times) — double-billing risk", bHits.Load())
	}
	if aHits.Load() != 1 {
		t.Fatalf("timeout must not retry same upstream (a hit %d times)", aHits.Load())
	}
}

// TestRetryOnTimeoutEnabled: with retry_on_timeout=true the policy retries.
func TestRetryOnTimeoutEnabled(t *testing.T) {
	var aHits, bHits atomic.Int32
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		aHits.Add(1)
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer a.Close()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bHits.Add(1)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer b.Close()

	cfg := &config.Config{
		Timeout:        config.Duration{Duration: 50 * time.Millisecond},
		RetryOnTimeout: true,
	}
	rl, _ := testRelay(cfg)
	cands := []config.Upstream{
		{Name: "a", BaseURL: a.URL, APIKey: "k", Models: []string{"m"}},
		{Name: "b", BaseURL: b.URL, APIKey: "k", Models: []string{"m"}},
	}
	res, f := rl.Do(context.Background(), []byte(`{"model":"m"}`), "/v1/chat/completions", "m", "m", cands, nil, "rid-3")
	if f != nil {
		t.Fatalf("expected failover after timeout when enabled: %v", f)
	}
	defer res.Close()
	if res.Upstream != "b" || bHits.Load() != 1 {
		t.Fatalf("should failover to b, got upstream=%s hits=%d", res.Upstream, bHits.Load())
	}
}

// TestNoFailoverOn4xx: client errors pass through without failover.
func TestNoFailoverOn4xx(t *testing.T) {
	var bHits atomic.Int32
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad request"}}`))
	}))
	defer a.Close()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bHits.Add(1)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer b.Close()

	rl, _ := testRelay(&config.Config{})
	cands := []config.Upstream{
		{Name: "a", BaseURL: a.URL, APIKey: "k", Models: []string{"m"}, Retry: 1},
		{Name: "b", BaseURL: b.URL, APIKey: "k", Models: []string{"m"}},
	}
	_, f := rl.Do(context.Background(), []byte(`{"model":"m"}`), "/v1/chat/completions", "m", "m", cands, nil, "rid-4")
	if f == nil {
		t.Fatal("expected failure")
	}
	if f.PassResponse == nil {
		t.Fatalf("4xx must set PassResponse, got %+v", f)
	}
	f.Close()
	if bHits.Load() != 0 {
		t.Fatal("4xx must not failover")
	}
}

// TestClientCancelNotRetried: parent context cancelled → no retry.
func TestClientCancelNotRetried(t *testing.T) {
	var aHits atomic.Int32
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		aHits.Add(1)
	}))
	defer a.Close()
	rl, _ := testRelay(&config.Config{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, f := rl.Do(ctx, []byte(`{"model":"m"}`), "/v1/chat/completions", "m", "m",
		[]config.Upstream{{Name: "a", BaseURL: a.URL, APIKey: "k", Models: []string{"m"}, Retry: 3}}, nil, "")
	if f == nil || !f.Cancelled {
		t.Fatalf("expected cancelled failure, got %+v", f)
	}
	if aHits.Load() != 0 {
		t.Fatal("must not hit upstream after client cancel")
	}
	st, _ := ErrStatus(f)
	if st != 499 {
		t.Fatalf("cancelled should map to 499, got %d", st)
	}
}

// TestRequestParamOverride: alias-rule params are merged into the body.
func TestRequestParamOverride(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		gotBody = buf[:n]
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	rl, _ := testRelay(&config.Config{})
	res, f := rl.Do(context.Background(), []byte(`{"model":"m","temperature":0.9}`),
		"/v1/chat/completions", "m", "m",
		[]config.Upstream{{Name: "a", BaseURL: srv.URL, APIKey: "k", Models: []string{"m"}}},
		map[string]any{"temperature": 0.1}, "rid-5")
	if f != nil {
		t.Fatalf("unexpected failure: %v", f)
	}
	defer res.Close()
	if !contains(gotBody, `"temperature":0.1`) {
		t.Fatalf("override not applied: %s", gotBody)
	}
}

func contains(b []byte, s string) bool {
	return len(b) > 0 && (string(b) != "" && (len(s) == 0 || (len(b) >= len(s) && indexOf(b, s) >= 0)))
}

func indexOf(b []byte, s string) int {
	for i := 0; i+len(s) <= len(b); i++ {
		if string(b[i:i+len(s)]) == s {
			return i
		}
	}
	return -1
}
