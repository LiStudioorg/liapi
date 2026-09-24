package stats

import (
	"sync"
	"time"
)

// AuditEvent is one admin API access record (for the security audit log).
type AuditEvent struct {
	Time   string `json:"time"`
	IP     string `json:"ip"`
	Method string `json:"method"`
	Path   string `json:"path"`
	OK     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`
}

// Audit keeps a bounded ring of admin API events. Memory-only by design.
type Audit struct {
	mu     sync.Mutex
	events []AuditEvent
	max    int
}

func NewAudit(max int) *Audit {
	if max <= 0 {
		max = 500
	}
	return &Audit{max: max}
}

func (a *Audit) Add(e AuditEvent) {
	if e.Time == "" {
		e.Time = time.Now().Format(time.RFC3339)
	}
	a.mu.Lock()
	a.events = append(a.events, e)
	if len(a.events) > a.max {
		a.events = a.events[len(a.events)-a.max:]
	}
	a.mu.Unlock()
}

// Recent returns the newest n events, newest first.
func (a *Audit) Recent(n int) []AuditEvent {
	a.mu.Lock()
	defer a.mu.Unlock()
	if n <= 0 || n > len(a.events) {
		n = len(a.events)
	}
	out := make([]AuditEvent, n)
	for i := 0; i < n; i++ {
		out[i] = a.events[len(a.events)-1-i]
	}
	return out
}
