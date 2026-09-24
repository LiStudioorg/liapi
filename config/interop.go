package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Export returns the full config as pretty JSON (for admin backup download).
// Secrets are included — the admin API must gate access.
func Export(c *Config) ([]byte, error) {
	return json.MarshalIndent(c, "", "  ")
}

// Import parses config JSON, applies defaults + validation, and returns the
// ready-to-swap config. It does NOT touch the holder or disk.
func Import(data []byte) (*Config, error) {
	cfg := &Config{}
	cfg.SetDefaults()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// LoadExport is a convenience: read a file and Import it.
func LoadExport(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Import(data)
}

// SaveExport writes cfg to path with 0600 (same as Save).
func SaveExport(path string, c *Config) error {
	return Save(path, c)
}

// ---------------------------------------------------------------------------
// OneAPI / new-api interop
// ---------------------------------------------------------------------------

// oneAPIChannel mirrors the subset of OneAPI's channel JSON we care about.
type oneAPIChannel struct {
	ID       int64                      `json:"id"`
	Name     string                     `json:"name"`
	Type     int                        `json:"type"`
	Key      string                     `json:"key"`
	Models   []string                   `json:"models"`
	BaseURL  string                     `json:"base_url"`
	Priority int                        `json:"priority"`
	Weight   int                        `json:"weight"`
	Status   int                        `json:"status"`
	Other    map[string]json.RawMessage `json:"-"`
}

type oneAPIExport struct {
	Channels []oneAPIChannel `json:"channels"`
	Data     []oneAPIChannel `json:"data"`
}

// BaseURLByType maps OneAPI channel type ids to default base URLs for the
// well-known providers. 0/empty base_url falls back to these.
var BaseURLByType = map[int]string{
	1:  "https://api.openai.com/v1",    // OpenAI
	3:  "https://api.openai.com/v1",    // Azure (approx)
	14: "https://api.anthropic.com/v1", // Anthropic
	11: "https://openrouter.ai/api/v1", // OpenRouter (approx)
	24: "https://api.mistral.ai/v1",    // Mistral (approx)
	15: "https://api.cohere.ai/v1",     // Cohere (approx)
}

// FromOneAPI converts a OneAPI/new-api channels export into a liapi Config.
// Only upstreams are populated; admin/client tokens, addr, etc. keep the
// caller's current values (merge with Import semantics).
//
// Input accepts either {"channels":[...]} / {"data":[...]} or a bare array.
func FromOneAPI(data []byte) ([]Upstream, error) {
	trim := strings.TrimSpace(string(data))
	var chans []oneAPIChannel
	if strings.HasPrefix(trim, "[") {
		if err := json.Unmarshal(data, &chans); err != nil {
			return nil, fmt.Errorf("parse oneapi channels: %w", err)
		}
	} else {
		var exp oneAPIExport
		if err := json.Unmarshal(data, &exp); err != nil {
			return nil, fmt.Errorf("parse oneapi export: %w", err)
		}
		chans = exp.Channels
		if len(chans) == 0 {
			chans = exp.Data
		}
	}
	if len(chans) == 0 {
		return nil, fmt.Errorf("oneapi export contains no channels")
	}

	out := make([]Upstream, 0, len(chans))
	seen := map[string]bool{}
	for _, ch := range chans {
		name := strings.TrimSpace(ch.Name)
		if name == "" {
			name = fmt.Sprintf("oneapi-%d", ch.ID)
		}
		// Dedup names.
		base := name
		for i := 2; seen[name]; i++ {
			name = fmt.Sprintf("%s-%d", base, i)
		}
		seen[name] = true

		baseURL := strings.TrimSpace(ch.BaseURL)
		if baseURL == "" {
			baseURL = BaseURLByType[ch.Type]
		}
		baseURL = strings.TrimRight(baseURL, "/")
		if baseURL == "" {
			return nil, fmt.Errorf("channel %q: cannot determine base_url (set base_url or known type)", name)
		}
		key := strings.TrimSpace(ch.Key)
		if key == "" {
			return nil, fmt.Errorf("channel %q: empty key", name)
		}
		// OneAPI packs multiple keys comma-separated; take the first.
		if i := strings.IndexByte(key, ','); i > 0 {
			key = strings.TrimSpace(key[:i])
		}
		models := ch.Models
		if len(models) == 0 {
			models = []string{"*"}
		}
		prio := ch.Priority
		if prio < 0 {
			prio = 0
		}
		w := ch.Weight
		if w < 0 {
			w = 0
		}
		disabled := ch.Status != 1 && ch.Status != 0 && ch.Status != -1 // 1=enabled in OneAPI; 0 often enabled draft
		// Status: OneAPI uses 1=enabled, 2=disabled (manually). Treat only 2 as disabled.
		disabled = ch.Status == 2

		out = append(out, Upstream{
			Name:     name,
			BaseURL:  baseURL,
			APIKey:   key,
			Models:   models,
			Priority: prio,
			Weight:   w,
			Disabled: disabled,
		})
	}
	return out, nil
}

// ToOneAPI renders the current upstreams as a OneAPI-like channels array
// (best-effort export for migration the other way).
func ToOneAPI(c *Config) ([]byte, error) {
	type chanOut struct {
		ID       int64    `json:"id"`
		Name     string   `json:"name"`
		Type     int      `json:"type"`
		Key      string   `json:"key"`
		Models   []string `json:"models"`
		BaseURL  string   `json:"base_url"`
		Priority int      `json:"priority"`
		Weight   int      `json:"weight"`
		Status   int      `json:"status"`
	}
	out := make([]chanOut, 0, len(c.Upstreams))
	for i, u := range c.Upstreams {
		status := 1
		if u.Disabled {
			status = 2
		}
		typ := 1 // default OpenAI-compatible
		switch {
		case strings.Contains(u.BaseURL, "anthropic"):
			typ = 14
		case strings.Contains(u.BaseURL, "openrouter"):
			typ = 11
		case strings.Contains(u.BaseURL, "mistral"):
			typ = 24
		case strings.Contains(u.BaseURL, "cohere"):
			typ = 15
		}
		out = append(out, chanOut{
			ID:       int64(i + 1),
			Name:     u.Name,
			Type:     typ,
			Key:      u.APIKey,
			Models:   u.Models,
			BaseURL:  u.BaseURL,
			Priority: u.Priority,
			Weight:   u.Weight,
			Status:   status,
		})
	}
	return json.MarshalIndent(out, "", "  ")
}
