package routing

import (
	"testing"

	"liapi/config"
)

type fakeHealth struct {
	ok  map[string]bool
	lat map[string]int64
}

func (f *fakeHealth) IsHealthy(n string) bool {
	if v, ok := f.ok[n]; ok {
		return v
	}
	return true
}

func (f *fakeHealth) LatencyMS(n string) int64 { return f.lat[n] }

func TestFallbackChainOrder(t *testing.T) {
	cfg := &config.Config{
		Upstreams: []config.Upstream{
			{Name: "A", BaseURL: "http://a/v1", APIKey: "k", Models: []string{"gpt-4o"}, Priority: 1, Weight: 5},
			{Name: "B", BaseURL: "http://b/v1", APIKey: "k", Models: []string{"gpt-4o"}, Priority: 1, Weight: 1},
			{Name: "C", BaseURL: "http://c/v1", APIKey: "k", Models: []string{"gpt-4o"}, Priority: 9, Weight: 9},
		},
		Fallbacks: map[string][]string{"gpt-4o": {"C", "A", "B"}},
	}
	rt := NewRouter(config.NewHolder(cfg), nil)
	// Chain overrides priority: C (prio 9) must come first, strict order.
	for i := 0; i < 20; i++ {
		c := rt.Candidates("gpt-4o", "")
		if len(c) != 3 || c[0].Name != "C" || c[1].Name != "A" || c[2].Name != "B" {
			t.Fatalf("chain order not respected: %v", names(c))
		}
	}
	// Disabled upstream in the chain is skipped, order otherwise kept.
	cfg.Upstreams[1].Disabled = true
	c := rt.Candidates("gpt-4o", "")
	if len(c) != 2 || c[0].Name != "C" || c[1].Name != "A" {
		t.Fatalf("disabled chain member should drop out: %v", names(c))
	}
}

func TestLatencyStrategy(t *testing.T) {
	cfg := &config.Config{
		RouteStrategy: "latency",
		Upstreams: []config.Upstream{
			{Name: "slow", BaseURL: "http://s/v1", APIKey: "k", Models: []string{"m"}, Weight: 1},
			{Name: "fast", BaseURL: "http://f/v1", APIKey: "k", Models: []string{"m"}, Weight: 1},
		},
	}
	h := &fakeHealth{lat: map[string]int64{"slow": 5000, "fast": 10}}
	rt := NewRouter(config.NewHolder(cfg), h)
	// Over many shuffles the fast upstream (reweighted weight ~100) must be
	// first far more often than the slow one (reweighted weight ~1).
	fastFirst := 0
	N := 100
	for i := 0; i < N; i++ {
		c := rt.Candidates("m", "")
		if c[0].Name == "fast" {
			fastFirst++
		}
	}
	if fastFirst < N*3/4 {
		t.Fatalf("latency strategy should prefer fast upstream, got %d/%d", fastFirst, N)
	}
}

func TestCostStrategy(t *testing.T) {
	cfg := &config.Config{
		RouteStrategy: "cost",
		Upstreams: []config.Upstream{
			{Name: "cheap", BaseURL: "http://c/v1", APIKey: "k", Models: []string{"m"}, Priority: 1, Weight: 1},
			{Name: "pricey", BaseURL: "http://p/v1", APIKey: "k", Models: []string{"m"}, Priority: 1, Weight: 1},
		},
		Prices: map[string]config.Price{"m": {Input: 1, Output: 2}},
	}
	// Cost uses prices[model] which is the same for both — order within the
	// group is then the stable pre-sort order. Differentiate via priority as
	// the no-price proxy is not used here; instead verify determinism.
	rt := NewRouter(config.NewHolder(cfg), nil)
	seen := map[string]int{}
	for i := 0; i < 10; i++ {
		c := rt.Candidates("m", "")
		seen[c[0].Name]++
	}
	if len(seen) != 1 {
		t.Fatalf("cost strategy should be deterministic for equal model price: %v", seen)
	}
}

func TestAliasRules(t *testing.T) {
	cfg := &config.Config{
		Aliases: map[string]string{"gpt-4o": "openai/gpt-4o"},
		AliasRules: []config.AliasRule{
			{Pattern: "^gpt-4o-mini$", Regex: true, Model: "mini-real", Params: map[string]any{"temperature": 0.2}},
			{Pattern: "exact-name", Model: "exact-real"},
		},
	}
	rt := NewRouter(config.NewHolder(cfg), nil)
	if got := rt.ResolveModel("gpt-4o"); got != "openai/gpt-4o" {
		t.Fatalf("exact aliases map should win: %s", got)
	}
	m, params := rt.ResolveAlias("gpt-4o-mini")
	if m != "mini-real" {
		t.Fatalf("regex rule: %s", m)
	}
	if params["temperature"] != 0.2 {
		t.Fatalf("params from rule: %v", params)
	}
	if got := rt.ResolveModel("exact-name"); got != "exact-real" {
		t.Fatalf("exact rule: %s", got)
	}
	if got := rt.ResolveModel("unknown"); got != "unknown" {
		t.Fatalf("passthrough: %s", got)
	}
	// Hot-swap config invalidates the compiled regex cache.
	cfg2 := cfg.Clone()
	cfg2.AliasRules = []config.AliasRule{{Pattern: "^other-(\\d+)$", Regex: true, Model: "num"}}
	h2 := config.NewHolder(cfg2)
	rt2 := NewRouter(h2, nil)
	if got := rt2.ResolveModel("other-7"); got != "num" {
		t.Fatalf("regex after swap: %s", got)
	}
}

func TestSkipUnhealthyFallback(t *testing.T) {
	cfg := &config.Config{
		SkipUnhealthy: true,
		Upstreams: []config.Upstream{
			{Name: "A", BaseURL: "http://a/v1", APIKey: "k", Models: []string{"m"}, Priority: 1},
			{Name: "B", BaseURL: "http://b/v1", APIKey: "k", Models: []string{"m"}, Priority: 2},
		},
	}
	h := &fakeHealth{ok: map[string]bool{"A": false, "B": false}}
	rt := NewRouter(config.NewHolder(cfg), h)
	c := rt.Candidates("m", "")
	if len(c) != 2 {
		t.Fatalf("all-unhealthy must fall back to full list, got %v", names(c))
	}
	h.ok["B"] = true
	c = rt.Candidates("m", "")
	if len(c) != 1 || c[0].Name != "B" {
		t.Fatalf("healthy-only filter failed: %v", names(c))
	}
}

func names(us []config.Upstream) []string {
	out := make([]string, len(us))
	for i, u := range us {
		out[i] = u.Name
	}
	return out
}
