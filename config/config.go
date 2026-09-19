package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"liapi/common"
)

// Duration is a time.Duration that unmarshals from either a JSON number
// (seconds) or a string (e.g. "30s").
type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if len(b) >= 2 && b[0] == '"' {
		v, err := time.ParseDuration(s)
		if err != nil {
			return err
		}
		d.Duration = v
		return nil
	}
	var secs float64
	if err := json.Unmarshal([]byte(s), &secs); err != nil {
		return err
	}
	d.Duration = time.Duration(secs * float64(time.Second))
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.Duration.Seconds())
}

// Price is a per-model price in yuan per 1M tokens (P2 cost estimation).
type Price struct {
	Input  float64 `json:"input"`
	Output float64 `json:"output"`
}

type Upstream struct {
	Name        string   `json:"name"`
	BaseURL     string   `json:"base_url"`
	APIKey      string   `json:"api_key"`
	Models      []string `json:"models"`
	Priority    int      `json:"priority"`
	Weight      int      `json:"weight"`
	Disabled    bool     `json:"disabled"`
	HealthPath  string   `json:"health_path"`
	Retry       int      `json:"retry"`
	InjectUsage bool     `json:"inject_usage"`
}

func (u *Upstream) Enabled() bool { return !u.Disabled }

func (u *Upstream) EffectiveHealthPath() string {
	if u.HealthPath != "" {
		return u.HealthPath
	}
	return "/v1/models"
}

func (u *Upstream) Matches(model string) bool {
	for _, m := range u.Models {
		if m == "*" || m == model {
			return true
		}
	}
	return false
}

type Config struct {
	Addr               string            `json:"addr"`
	BodyLimitBytes     int64             `json:"body_limit_bytes"`
	Timeout            Duration          `json:"timeout"`
	StreamTimeout      Duration          `json:"stream_timeout"`
	MaxIdleConns       int               `json:"max_idle_conns"`
	LogFile            string            `json:"log_file"`
	RingSize           int               `json:"ring_size"`
	ProbeInterval      Duration          `json:"probe_interval"`
	ProbeTimeout       Duration          `json:"probe_timeout"`
	ProbeFailThreshold int               `json:"probe_fail_threshold"`
	SkipUnhealthy      bool              `json:"skip_unhealthy"`
	AdminToken         string            `json:"admin_token"`
	ClientTokens       []string          `json:"client_tokens"`
	RateLimitPerMinute int               `json:"rate_limit_per_minute"`
	Aliases            map[string]string `json:"aliases"`
	Prices             map[string]Price  `json:"prices"`
	Upstreams          []Upstream        `json:"upstreams"`
}

func (c *Config) SetDefaults() {
	if c.Addr == "" {
		c.Addr = ":8787"
	}
	if c.BodyLimitBytes <= 0 {
		c.BodyLimitBytes = 16 << 20
	}
	if c.Timeout.Duration <= 0 {
		c.Timeout.Duration = 300 * time.Second
	}
	if c.MaxIdleConns <= 0 {
		c.MaxIdleConns = 100
	}
	if c.LogFile == "" {
		c.LogFile = "relay.jsonl"
	}
	if c.RingSize < 100 {
		c.RingSize = 800
	}
	if c.ProbeInterval.Duration <= 0 {
		c.ProbeInterval.Duration = 30 * time.Second
	}
	if c.ProbeTimeout.Duration <= 0 {
		c.ProbeTimeout.Duration = 5 * time.Second
	}
	if c.ProbeFailThreshold <= 0 {
		c.ProbeFailThreshold = 3
	}
}

func (c *Config) Validate() error {
	for i := range c.Upstreams {
		u := &c.Upstreams[i]
		u.Name = strings.TrimSpace(u.Name)
		u.BaseURL = strings.TrimRight(strings.TrimSpace(u.BaseURL), "/")
		u.HealthPath = strings.TrimSpace(u.HealthPath)
		if u.Weight <= 0 {
			u.Weight = 1
		}
	}

	names := map[string]bool{}
	for i := range c.Upstreams {
		u := &c.Upstreams[i]
		if u.Name == "" {
			return errors.New("upstream: name is required")
		}
		if names[u.Name] {
			return fmt.Errorf("upstream %q: duplicate name", u.Name)
		}
		names[u.Name] = true
		if u.BaseURL == "" {
			return fmt.Errorf("upstream %q: base_url is required", u.Name)
		}
		parsed, err := url.Parse(u.BaseURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return fmt.Errorf("upstream %q: invalid base_url %q", u.Name, u.BaseURL)
		}
		if u.APIKey == "" && !u.Disabled {
			return fmt.Errorf("upstream %q: api_key is required (or set disabled=true)", u.Name)
		}
		if len(u.Models) == 0 {
			return fmt.Errorf("upstream %q: models list is required (use [\"*\"] for catch-all)", u.Name)
		}
	}

	if c.AdminToken == "" {
		c.AdminToken = common.RandomToken("adm-")
	}

	seen := map[string]bool{}
	var tokens []string
	for _, t := range c.ClientTokens {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		tokens = append(tokens, t)
	}
	c.ClientTokens = tokens
	return nil
}

// Clone returns a deep copy of the config.
func (c *Config) Clone() *Config {
	data, err := json.Marshal(c)
	if err != nil {
		return c
	}
	out := &Config{}
	_ = json.Unmarshal(data, out)
	out.SetDefaults()
	return out
}

// Load reads the config file. If it does not exist, a default config is
// generated (with a random admin token) and saved.
func Load(path string) (*Config, error) {
	if path == "" {
		cfg := &Config{}
		cfg.SetDefaults()
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		cfg := &Config{}
		cfg.SetDefaults()
		if err := cfg.Validate(); err != nil {
			return nil, err
		}
		if err := Save(path, cfg); err != nil {
			return nil, fmt.Errorf("create default config: %w", err)
		}
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	cfg := &Config{}
	cfg.SetDefaults()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Save writes the config atomically: temp file in the same dir + rename.
func Save(path string, c *Config) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".liapi-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}
