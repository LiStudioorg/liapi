package stats

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"

	"liapi/config"
)

// HealthStatus is the per-upstream health check result.
type HealthStatus struct {
	Name                string `json:"name"`
	OK                  bool   `json:"ok"`
	LatencyMS           int64  `json:"latency_ms"`
	LastCheck           string `json:"last_check,omitempty"`
	Error               string `json:"error,omitempty"`
	ConsecutiveFailures int    `json:"consecutive_failures"`
}

// Health background-probes enabled upstreams and stores results in memory.
// An upstream is only marked unhealthy after probe_fail_threshold
// consecutive failures (avoids flapping).
type Health struct {
	holder *config.Holder
	client *http.Client
	mu     sync.RWMutex
	states map[string]*HealthStatus
}

func NewHealth(holder *config.Holder, client *http.Client) *Health {
	return &Health{holder: holder, client: client, states: make(map[string]*HealthStatus)}
}

func (h *Health) Start(ctx context.Context) {
	interval := h.holder.Get().ProbeInterval.Duration
	if interval <= 0 {
		interval = 30 * time.Second
	}
	go func() {
		h.probeAll()
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				h.probeAll()
			case <-ctx.Done():
				return
			}
		}
	}()
}

// IsHealthy returns true when the upstream should be routed to.
// Unknown upstreams (not yet probed) are considered healthy.
func (h *Health) IsHealthy(name string) bool {
	threshold := h.holder.Get().ProbeFailThreshold
	if threshold <= 0 {
		threshold = 3
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	s := h.states[name]
	if s == nil {
		return true
	}
	if !s.OK {
		return s.ConsecutiveFailures < threshold
	}
	return true
}

func (h *Health) All() []HealthStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]HealthStatus, 0, len(h.states))
	for _, s := range h.states {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (h *Health) probeAll() {
	cfg := h.holder.Get()
	timeout := cfg.ProbeTimeout.Duration
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	threshold := cfg.ProbeFailThreshold
	if threshold <= 0 {
		threshold = 3
	}

	var wg sync.WaitGroup
	for i := range cfg.Upstreams {
		up := &cfg.Upstreams[i]
		if !up.Enabled() {
			continue
		}
		wg.Add(1)
		go func(u *config.Upstream) {
			defer wg.Done()
			raw := h.probe(u, timeout)
			st := HealthStatus{
				Name:      u.Name,
				OK:        raw.OK,
				LatencyMS: raw.LatencyMS,
				LastCheck: time.Now().Format(time.RFC3339),
				Error:     raw.Error,
			}
			prevFail := h.consecutive(u.Name)
			if st.OK {
				st.ConsecutiveFailures = 0
			} else {
				st.ConsecutiveFailures = prevFail + 1
			}
			_ = threshold
			h.set(u.Name, &st)
		}(up)
	}
	wg.Wait()
}

func (h *Health) probe(u *config.Upstream, timeout time.Duration) *HealthStatus {
	st := &HealthStatus{Name: u.Name}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	url := u.BaseURL + u.EffectiveHealthPath()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		st.Error = err.Error()
		return st
	}
	if u.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+u.APIKey)
	}
	start := time.Now()
	resp, err := h.client.Do(req)
	st.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		st.Error = err.Error()
		return st
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 500 {
		st.OK = true
	} else {
		st.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return st
}

func (h *Health) consecutive(name string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if s := h.states[name]; s != nil {
		return s.ConsecutiveFailures
	}
	return 0
}

func (h *Health) set(name string, st *HealthStatus) {
	h.mu.Lock()
	h.states[name] = st
	h.mu.Unlock()
}
