package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
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

// TokenPolicy is an optional per-token restriction. Keyed by token value (or
// "dev:<device id>" for devices) in Config.TokenPolicies.
type TokenPolicy struct {
	ExpiresAt string   `json:"expires_at,omitempty"` // RFC3339; empty = never expires
	AllowIPs  []string `json:"allow_ips,omitempty"`  // empty = any IP
	RPM       int      `json:"rpm,omitempty"`        // 0 = use global/device limit
}

// AlertConfig selects where operational alerts (failover / unhealthy /
// quota) are delivered. All destinations receive a JSON POST.
type AlertConfig struct {
	Webhooks          []string `json:"webhooks,omitempty"`
	Bark              []string `json:"bark,omitempty"`
	OnFailover        bool     `json:"on_failover"`
	OnUnhealthy       bool     `json:"on_unhealthy"`
	OnQuota           bool     `json:"on_quota"`
	QuotaWarnPercent  int      `json:"quota_warn_percent"`         // warn when usage crosses this % (default 80)
	MinAlertIntervalS int      `json:"min_alert_interval_seconds"` // dedup window, default 60
}

type Upstream struct {
	Name        string            `json:"name"`
	BaseURL     string            `json:"base_url"`
	APIKey      string            `json:"api_key"`
	Models      []string          `json:"models"`
	Priority    int               `json:"priority"`
	Weight      int               `json:"weight"`
	Disabled    bool              `json:"disabled"`
	HealthPath  string            `json:"health_path"`
	Retry       int               `json:"retry"`
	InjectUsage bool              `json:"inject_usage"`
	Group       string            `json:"group,omitempty"`     // channel group label (display only)
	ModelMap    map[string]string `json:"model_map,omitempty"` // resolved model -> upstream model name
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

// Device is a per-client credential. The plaintext token is shown exactly
// once at creation; only its SHA-256 hash is stored.
type Device struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	TokenHash string `json:"token_hash"` // sha256 hex, never the raw token
	RPM       int    `json:"rpm"`        // per-minute limit, 0 = use global
	Daily     int    `json:"daily"`      // per-day limit, 0 = use global
	Disabled  bool   `json:"disabled"`
	CreatedAt string `json:"created_at,omitempty"`
	Note      string `json:"note,omitempty"`
}

// AliasRule rewrites model names by exact match or regex and can force
// extra parameters onto the outbound body.
type AliasRule struct {
	Pattern string         `json:"pattern"`
	Regex   bool           `json:"regex"`
	Model   string         `json:"model"`
	Params  map[string]any `json:"params,omitempty"`
}

type Config struct {
	Addr               string                 `json:"addr"`
	BodyLimitBytes     int64                  `json:"body_limit_bytes"`
	Timeout            Duration               `json:"timeout"`
	StreamTimeout      Duration               `json:"stream_timeout"`
	StreamIdleTimeout  Duration               `json:"stream_idle_timeout"`
	MaxIdleConns       int                    `json:"max_idle_conns"`
	LogFile            string                 `json:"log_file"`
	LogMaxBytes        int64                  `json:"log_max_bytes"`
	RingSize           int                    `json:"ring_size"`
	ProbeInterval      Duration               `json:"probe_interval"`
	ProbeTimeout       Duration               `json:"probe_timeout"`
	ProbeFailThreshold int                    `json:"probe_fail_threshold"`
	ProbeConcurrency   int                    `json:"probe_concurrency"`
	HealthStateFile    string                 `json:"health_state_file"`
	SkipUnhealthy      bool                   `json:"skip_unhealthy"`
	AdminToken         string                 `json:"admin_token"`
	AdminRatePerMinute int                    `json:"admin_rate_per_minute"`
	AdminAllowIPs      []string               `json:"admin_allow_ips"`
	MetricsToken       string                 `json:"metrics_token"`
	ClientTokens       []string               `json:"client_tokens"`
	RateLimitPerMinute int                    `json:"rate_limit_per_minute"`
	DailyPerToken      int                    `json:"daily_per_token"`
	TokenQuotas        map[string]int         `json:"token_quotas"`
	TokenPolicies      map[string]TokenPolicy `json:"token_policies"`
	Alerts             AlertConfig            `json:"alerts"`
	LogRetentionDays   int                    `json:"log_retention_days"` // purge rotated daily logs older than N days; 0 = keep forever
	LogRawTokens       bool                   `json:"log_raw_tokens"`     // false (default) masks tokens in JSONL logs
	Aliases            map[string]string      `json:"aliases"`
	AliasRules         []AliasRule            `json:"alias_rules"`
	Prices             map[string]Price       `json:"prices"`
	RouteStrategy      string                 `json:"route_strategy"` // "" | latency | cost
	Fallbacks          map[string][]string    `json:"fallbacks"`      // model -> ordered upstream names
	RetryOnTimeout     bool                   `json:"retry_on_timeout"`
	Upstreams          []Upstream             `json:"upstreams"`
	Devices            []Device               `json:"devices"`
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
	if c.LogMaxBytes <= 0 {
		c.LogMaxBytes = 100 << 20
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
	if c.ProbeConcurrency <= 0 {
		c.ProbeConcurrency = 8
	}
	if c.HealthStateFile == "" {
		c.HealthStateFile = "health_state.json"
	}
	if c.AdminRatePerMinute == 0 {
		c.AdminRatePerMinute = 60 // 0 = unset → default; -1 = explicitly disabled
	}
	if c.StreamIdleTimeout.Duration == 0 {
		c.StreamIdleTimeout.Duration = 10 * time.Minute
	}
	if c.Alerts.QuotaWarnPercent == 0 {
		c.Alerts.QuotaWarnPercent = 80
	}
	if c.Alerts.MinAlertIntervalS == 0 {
		c.Alerts.MinAlertIntervalS = 60
	}
}

func (c *Config) Validate() error {
	for i := range c.Upstreams {
		u := &c.Upstreams[i]
		u.Name = strings.TrimSpace(u.Name)
		u.BaseURL = strings.TrimRight(strings.TrimSpace(u.BaseURL), "/")
		u.HealthPath = strings.TrimSpace(u.HealthPath)
		if u.Weight < 0 {
			return fmt.Errorf("upstream %q: weight must be >= 0", u.Name)
		}
		if u.Weight == 0 {
			u.Weight = 1
		}
		if u.Retry < 0 || u.Retry > 10 {
			return fmt.Errorf("upstream %q: retry must be in [0,10]", u.Name)
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
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return fmt.Errorf("upstream %q: base_url scheme must be http/https", u.Name)
		}
		if u.APIKey == "" && !u.Disabled {
			return fmt.Errorf("upstream %q: api_key is required (or set disabled=true)", u.Name)
		}
		if len(u.Models) == 0 {
			return fmt.Errorf("upstream %q: models list is required (use [\"*\"] for catch-all)", u.Name)
		}
		for from, to := range u.ModelMap {
			if strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" {
				return fmt.Errorf("upstream %q: model_map keys/values must be non-empty", u.Name)
			}
		}
	}

	if c.AdminToken == "" {
		c.AdminToken = common.RandomToken("adm-")
	}
	if c.RateLimitPerMinute < 0 {
		return errors.New("rate_limit_per_minute must be >= 0")
	}
	if c.AdminRatePerMinute < -1 {
		return errors.New("admin_rate_per_minute must be >= -1 (-1 disables, 0 means default 60)")
	}
	if c.DailyPerToken < 0 {
		return errors.New("daily_per_token must be >= 0")
	}
	switch c.RouteStrategy {
	case "", "priority", "latency", "cost":
	default:
		return fmt.Errorf("route_strategy must be priority|latency|cost, got %q", c.RouteStrategy)
	}

	// Fallback chains reference existing upstreams by name.
	for model, chain := range c.Fallbacks {
		if len(chain) == 0 {
			return fmt.Errorf("fallbacks[%q]: empty chain", model)
		}
		seen := map[string]bool{}
		for _, name := range chain {
			if !names[name] {
				return fmt.Errorf("fallbacks[%q]: unknown upstream %q", model, name)
			}
			if seen[name] {
				return fmt.Errorf("fallbacks[%q]: duplicate upstream %q", model, name)
			}
			seen[name] = true
		}
	}

	// Token policies: expiry must parse when set; rpm >= 0.
	for k, p := range c.TokenPolicies {
		if p.ExpiresAt != "" {
			if _, err := time.Parse(time.RFC3339, p.ExpiresAt); err != nil {
				return fmt.Errorf("token_policies[%q]: invalid expires_at (want RFC3339): %v", k, err)
			}
		}
		if p.RPM < 0 {
			return fmt.Errorf("token_policies[%q]: rpm must be >= 0", k)
		}
	}
	if c.Alerts.QuotaWarnPercent < 0 || c.Alerts.QuotaWarnPercent > 100 {
		return errors.New("alerts.quota_warn_percent must be in [0,100]")
	}
	if c.LogRetentionDays < 0 {
		return errors.New("log_retention_days must be >= 0")
	}

	// Alias rules: regex must compile, model required.
	for i, r := range c.AliasRules {
		if strings.TrimSpace(r.Pattern) == "" {
			return fmt.Errorf("alias_rules[%d]: pattern is required", i)
		}
		if strings.TrimSpace(r.Model) == "" {
			return fmt.Errorf("alias_rules[%d]: model is required", i)
		}
		if r.Regex {
			if _, err := regexp.Compile(r.Pattern); err != nil {
				return fmt.Errorf("alias_rules[%d]: invalid regex: %w", i, err)
			}
		}
	}

	// Devices: unique id + hash.
	devIDs := map[string]bool{}
	devHash := map[string]bool{}
	for i := range c.Devices {
		d := &c.Devices[i]
		if d.ID == "" {
			return fmt.Errorf("devices[%d]: id is required", i)
		}
		if devIDs[d.ID] {
			return fmt.Errorf("devices[%d]: duplicate id %q", i, d.ID)
		}
		devIDs[d.ID] = true
		if d.TokenHash == "" {
			return fmt.Errorf("device %q: token_hash is required", d.ID)
		}
		if devHash[d.TokenHash] {
			return fmt.Errorf("device %q: duplicate token_hash", d.ID)
		}
		devHash[d.TokenHash] = true
		if d.RPM < 0 || d.Daily < 0 {
			return fmt.Errorf("device %q: rpm/daily must be >= 0", d.ID)
		}
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
	if err := checkFilePerm(path); err != nil {
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

// checkFilePerm refuses world/group-readable configs (they hold API keys).
// Windows has no POSIX bits; skip there. Set LIAPI_SKIP_PERM_CHECK=1 to bypass.
func checkFilePerm(path string) error {
	if os.Getenv("LIAPI_SKIP_PERM_CHECK") == "1" {
		return nil
	}
	fi, err := os.Stat(path)
	if err != nil {
		return nil // best effort; Load already read the file
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("config file %s has mode %#o; must be 0600 (contains secrets). Run: chmod 600 %s", path, fi.Mode().Perm(), path)
	}
	return nil
}

// Save writes the config atomically: temp file in the same dir + rename.
// The file is forced to 0600.
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
	_ = tmp.Chmod(0o600)
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
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	// Rename may land with temp defaults on some FS; enforce after rename.
	_ = os.Chmod(path, 0o600)
	return nil
}
