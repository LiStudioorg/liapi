package config

import "sync"

// Holder is the single source of truth for the current config. All requests
// read from it live (hot reload), never a cached copy.
type Holder struct {
	mu   sync.RWMutex
	cfg  *Config
	prev *Config // last known-good, kept for rollback reference
}

func NewHolder(cfg *Config) *Holder {
	return &Holder{cfg: cfg}
}

func (h *Holder) Get() *Config {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.cfg
}

// Prev returns the previous known-good config (nil before first failed swap).
func (h *Holder) Prev() *Config {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.prev
}

// Set validates cfg first; on validation failure the old config is kept
// unchanged and the error is returned (atomic swap, never a partial replace).
func (h *Holder) Set(cfg *Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	h.mu.Lock()
	h.prev = h.cfg
	h.cfg = cfg
	h.mu.Unlock()
	return nil
}
