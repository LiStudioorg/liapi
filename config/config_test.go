package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDurationJSON(t *testing.T) {
	var d Duration
	if err := json.Unmarshal([]byte(`30`), &d); err != nil {
		t.Fatal(err)
	}
	if d.Duration != 30*time.Second {
		t.Fatalf("numeric duration: got %v", d.Duration)
	}
	if err := json.Unmarshal([]byte(`"45s"`), &d); err != nil {
		t.Fatal(err)
	}
	if d.Duration != 45*time.Second {
		t.Fatalf("string duration: got %v", d.Duration)
	}
	if err := json.Unmarshal([]byte(`"bogus"`), &d); err == nil {
		t.Fatal("invalid duration should error")
	}
}

func validConfig() *Config {
	c := &Config{}
	c.SetDefaults()
	c.AdminToken = "adm-x"
	c.ClientTokens = []string{"sk-a", "sk-a", " sk-b ", ""}
	c.Upstreams = []Upstream{
		{Name: "A", BaseURL: "https://api.openai.com/v1", APIKey: "k", Models: []string{"gpt-4o"}},
	}
	return c
}

func TestValidateOK(t *testing.T) {
	c := validConfig()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(c.ClientTokens) != 2 {
		t.Fatalf("tokens not deduped/trimmed: %v", c.ClientTokens)
	}
}

func TestValidateErrors(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Config)
	}{
		{"dup upstream", func(c *Config) {
			c.Upstreams = append(c.Upstreams, c.Upstreams[0])
		}},
		{"bad url", func(c *Config) {
			c.Upstreams[0].BaseURL = "not-a-url"
		}},
		{"bad scheme", func(c *Config) {
			c.Upstreams[0].BaseURL = "ftp://x/v1"
		}},
		{"empty models", func(c *Config) {
			c.Upstreams[0].Models = nil
		}},
		{"neg weight", func(c *Config) {
			c.Upstreams[0].Weight = -1
		}},
		{"retry range", func(c *Config) {
			c.Upstreams[0].Retry = 99
		}},
		{"bad strategy", func(c *Config) {
			c.RouteStrategy = "magic"
		}},
		{"neg rate", func(c *Config) {
			c.RateLimitPerMinute = -1
		}},
		{"fallback unknown", func(c *Config) {
			c.Fallbacks = map[string][]string{"gpt-4o": {"nope"}}
		}},
		{"fallback dup", func(c *Config) {
			c.Fallbacks = map[string][]string{"gpt-4o": {"A", "A"}}
		}},
		{"alias regex", func(c *Config) {
			c.AliasRules = []AliasRule{{Pattern: "([", Regex: true, Model: "x"}}
		}},
		{"alias empty pattern", func(c *Config) {
			c.AliasRules = []AliasRule{{Pattern: "", Model: "x"}}
		}},
		{"device no hash", func(c *Config) {
			c.Devices = []Device{{ID: "d1"}}
		}},
		{"device dup id", func(c *Config) {
			c.Devices = []Device{
				{ID: "d1", TokenHash: "aa"},
				{ID: "d1", TokenHash: "bb"},
			}
		}},
	}
	for _, tc := range cases {
		c := validConfig()
		c.Upstreams[0].Name = "A"
		tc.mut(c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s: expected error", tc.name)
		}
	}
}

func TestLoadPermCheck(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := validConfig()
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	// Save forces 0600 — should load cleanly.
	if _, err := Load(path); err != nil {
		t.Fatalf("0600 config should load: %v", err)
	}
	// Loosen perms → must refuse (unless bypass env set).
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("0644 config must be rejected")
	}
	t.Setenv("LIAPI_SKIP_PERM_CHECK", "1")
	if _, err := Load(path); err != nil {
		t.Fatalf("bypass env should allow load: %v", err)
	}
}

func TestExportImportRoundtrip(t *testing.T) {
	cfg := validConfig()
	cfg.RouteStrategy = "latency"
	data, err := Export(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Import(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.RouteStrategy != "latency" || len(got.Upstreams) != 1 {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	if _, err := Import([]byte("{bad")); err == nil {
		t.Fatal("invalid JSON must error")
	}
	if _, err := Import([]byte(`{"upstreams":[{"name":"x"}]}`)); err == nil {
		t.Fatal("invalid config must error")
	}
}

func TestFromOneAPI(t *testing.T) {
	raw := `{"channels":[
		{"id":1,"name":"openai","type":1,"key":"sk-one,sk-two","models":["gpt-4o"],"base_url":"","priority":1,"weight":2,"status":1},
		{"id":2,"name":"ant","type":14,"key":"sk-ant","models":["claude-3-5-sonnet"],"base_url":"https://api.anthropic.com/v1","priority":2,"status":2}
	]}`
	ups, err := FromOneAPI([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(ups) != 2 {
		t.Fatalf("want 2 upstreams, got %d", len(ups))
	}
	if ups[0].BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("type->base_url mapping failed: %q", ups[0].BaseURL)
	}
	if ups[0].APIKey != "sk-one" {
		t.Fatalf("multi-key should take first: %q", ups[0].APIKey)
	}
	if ups[1].Disabled != true {
		t.Fatal("status=2 should be disabled")
	}
	// Bare array form.
	ups, err = FromOneAPI([]byte(`[{"id":9,"name":"local","key":"k","models":["*"],"base_url":"http://localhost:11434/v1","status":1}]`))
	if err != nil || len(ups) != 1 || ups[0].Name != "local" {
		t.Fatalf("bare array parse failed: %v %v", err, ups)
	}
	if _, err := FromOneAPI([]byte(`{"channels":[]}`)); err == nil {
		t.Fatal("empty channels must error")
	}
}

func TestHolderSetAtomic(t *testing.T) {
	good := validConfig()
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	h := NewHolder(good)
	bad := validConfig()
	bad.Upstreams[0].BaseURL = "nope"
	if err := h.Set(bad); err == nil {
		t.Fatal("Set must reject invalid config")
	}
	if h.Get() != good {
		t.Fatal("failed Set must keep old config")
	}
	better := validConfig()
	better.Addr = ":9999"
	if err := h.Set(better); err != nil {
		t.Fatal(err)
	}
	if h.Get().Addr != ":9999" || h.Prev() != good {
		t.Fatal("successful Set should swap and keep prev")
	}
}
