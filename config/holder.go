package config

import "sync"

// Holder is the single source of truth for the current config. All requests
// read from it live (hot reload), never a cached copy.
type Holder struct {
	mu  sync.RWMutex
	cfg *Config
}

func NewHolder(cfg *Config) *Holder {
	return &Holder{cfg: cfg}
}

func (h *Holder) Get() *Config {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.cfg
}

func (h *Holder) Set(cfg *Config) {
	h.mu.Lock()
	h.cfg = cfg
	h.mu.Unlock()
}
