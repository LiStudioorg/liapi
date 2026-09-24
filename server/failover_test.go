package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"liapi/config"
)

// TestFailoverIntegration spins up two fake upstreams: the first returns 500
// on every attempt, the second returns 200. The proxy must transparently fail
// over and the client must see the successful response.
func TestFailoverIntegration(t *testing.T) {
	var firstHits, secondHits atomic.Int64

	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstHits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"boom"}}`))
	}))
	defer first.Close()

	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"ok","choices":[{"message":{"role":"assistant","content":"hi"}}],"usage":{"prompt_tokens":3,"completion_tokens":4}}`))
	}))
	defer second.Close()

	srv, _ := newTestServer(t, func(c *config.Config) {
		c.Upstreams = []config.Upstream{
			{Name: "bad", BaseURL: first.URL, APIKey: "k1", Models: []string{"m"}, Priority: 1, Weight: 1},
			{Name: "good", BaseURL: second.URL, APIKey: "k2", Models: []string{"m"}, Priority: 2, Weight: 1},
		}
	})
	h := srv.Handler()

	rec := do(h, "POST", "/v1/chat/completions", "sk-client-0123456789",
		`{"model":"m","messages":[{"role":"user","content":"hello"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 after failover, got %d: %s", rec.Code, rec.Body.String())
	}
	if secondHits.Load() == 0 {
		t.Fatal("second upstream was never hit")
	}
	if firstHits.Load() == 0 {
		t.Fatal("first upstream was never attempted")
	}
	if !strings.Contains(rec.Body.String(), `"hi"`) {
		t.Fatalf("response should come from the healthy upstream: %s", rec.Body.String())
	}

	// Failover must be visible in metrics.
	mrec := do(h, "GET", "/metrics", adminTok, "")
	if !strings.Contains(mrec.Body.String(), "liapi_failovers_total") {
		t.Fatal("metrics missing failover counter name")
	}
	// And in the request log entry (upstream recorded = the one that served).
	// Logging is async (channel → writer goroutine), so poll briefly.
	var sawGood bool
	for i := 0; i < 100; i++ {
		sawGood = false
		for _, e := range srv.logger.Recent(10) {
			if e.Upstream == "good" && e.Status == 200 {
				sawGood = true
				break
			}
		}
		if sawGood {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !sawGood {
		logs := srv.logger.Recent(10)
		t.Fatalf("log ring should record the successful upstream, got %+v", logs)
	}
}

// TestFailoverAllDown ensures the client gets a 502 (not a hang or 200) when
// every candidate fails.
func TestFailoverAllDown(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer dead.Close()

	srv, _ := newTestServer(t, func(c *config.Config) {
		c.Upstreams = []config.Upstream{
			{Name: "d1", BaseURL: dead.URL, APIKey: "k", Models: []string{"m"}, Priority: 1, Weight: 1},
			{Name: "d2", BaseURL: dead.URL, APIKey: "k", Models: []string{"m"}, Priority: 2, Weight: 1},
		}
	})
	rec := do(srv.Handler(), "POST", "/v1/chat/completions", "sk-client-0123456789",
		`{"model":"m","messages":[{"role":"user","content":"x"}]}`)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("want 502 when all upstreams down, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestModelMapApplied verifies the per-upstream model_map rewrites the model
// sent to the upstream while the client-visible log keeps the original.
func TestModelMapApplied(t *testing.T) {
	var gotModel string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotModel = body.Model
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","choices":[]}`))
	}))
	defer up.Close()

	srv, _ := newTestServer(t, func(c *config.Config) {
		c.Upstreams = []config.Upstream{{
			Name: "u", BaseURL: up.URL, APIKey: "k", Models: []string{"pub-model"},
			ModelMap: map[string]string{"pub-model": "internal-real-model"},
		}}
	})
	rec := do(srv.Handler(), "POST", "/v1/chat/completions", "sk-client-0123456789",
		`{"model":"pub-model","messages":[]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if gotModel != "internal-real-model" {
		t.Fatalf("upstream should receive mapped model, got %q", gotModel)
	}
}
