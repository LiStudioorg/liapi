package stats

import "sync"

// TotalsSnapshot is the aggregate statistics exposed to the admin overview.
type TotalsSnapshot struct {
	Requests       int64   `json:"requests"`
	Success        int64   `json:"success"`
	ClientErrors   int64   `json:"client_errors"`
	UpstreamErrors int64   `json:"upstream_errors"`
	LatencyMS      int64   `json:"latency_ms"`
	InTokens       int64   `json:"in_tokens"`
	OutTokens      int64   `json:"out_tokens"`
	Multimodal     int64   `json:"multimodal"`
	Cost           float64 `json:"cost"`
}

// Totals accumulates all-time counters. It is only written from the single
// log goroutine path (Add is also called from handler defer), guarded by mutex.
type Totals struct {
	mu   sync.Mutex
	snap TotalsSnapshot
}

func NewTotals() *Totals { return &Totals{} }

func (t *Totals) Add(e Entry) {
	t.mu.Lock()
	s := &t.snap
	s.Requests++
	switch {
	case e.Status >= 200 && e.Status < 300:
		s.Success++
	case e.Status >= 400 && e.Status < 500:
		s.ClientErrors++
	default:
		s.UpstreamErrors++
	}
	s.LatencyMS += e.LatencyMS
	s.InTokens += int64(e.InTokens)
	s.OutTokens += int64(e.OutTokens)
	s.Multimodal += int64(e.Multimodal)
	s.Cost += e.Cost
	t.mu.Unlock()
}

func (t *Totals) Snapshot() TotalsSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.snap
}
