package routing

import (
	"testing"

	"liapi/config"
)

func TestCandidatesOrderByPriority(t *testing.T) {
	cfg := &config.Config{
		Upstreams: []config.Upstream{
			{Name: "A", BaseURL: "http://a/v1", APIKey: "k", Models: []string{"gpt-4o"}, Priority: 2, Weight: 1},
			{Name: "B", BaseURL: "http://b/v1", APIKey: "k", Models: []string{"gpt-4o"}, Priority: 1, Weight: 1},
			{Name: "C", BaseURL: "http://c/v1", APIKey: "k", Models: []string{"*"}, Priority: 3, Weight: 1},
			{Name: "D", BaseURL: "http://d/v1", APIKey: "k", Models: []string{"llama"}, Priority: 1, Weight: 1},
		},
	}
	holder := config.NewHolder(cfg)
	rt := NewRouter(holder, nil)

	c := rt.Candidates("gpt-4o", "")
	if len(c) != 3 {
		t.Fatalf("want 3 candidates, got %d", len(c))
	}
	if c[0].Name != "B" {
		t.Fatalf("B (priority 1) should be first, got %s", c[0].Name)
	}
	if c[2].Name != "C" {
		t.Fatalf("C (priority 3, wildcard) should be last, got %s", c[2].Name)
	}
}

func TestCandidatesNoMatch(t *testing.T) {
	cfg := &config.Config{
		Upstreams: []config.Upstream{
			{Name: "A", BaseURL: "http://a/v1", APIKey: "k", Models: []string{"gpt-4o"}, Priority: 1, Weight: 1},
		},
	}
	holder := config.NewHolder(cfg)
	rt := NewRouter(holder, nil)
	if c := rt.Candidates("gpt-3.5", ""); len(c) != 0 {
		t.Fatalf("want no candidates, got %d", len(c))
	}
}

func TestCandidatesSkipDisabled(t *testing.T) {
	cfg := &config.Config{
		Upstreams: []config.Upstream{
			{Name: "A", BaseURL: "http://a/v1", APIKey: "k", Models: []string{"gpt-4o"}, Priority: 1, Weight: 1, Disabled: true},
			{Name: "B", BaseURL: "http://b/v1", APIKey: "k", Models: []string{"gpt-4o"}, Priority: 2, Weight: 1},
		},
	}
	holder := config.NewHolder(cfg)
	rt := NewRouter(holder, nil)
	c := rt.Candidates("gpt-4o", "")
	if len(c) != 1 || c[0].Name != "B" {
		t.Fatalf("disabled upstream should be skipped, got %v", c)
	}
}

func TestAlias(t *testing.T) {
	cfg := &config.Config{Aliases: map[string]string{"gpt-4o": "openai/gpt-4o"}}
	holder := config.NewHolder(cfg)
	rt := NewRouter(holder, nil)
	if got := rt.ResolveModel("gpt-4o"); got != "openai/gpt-4o" {
		t.Fatalf("alias not resolved: %s", got)
	}
	if got := rt.ResolveModel("unknown"); got != "unknown" {
		t.Fatalf("unknown model should pass through: %s", got)
	}
}
