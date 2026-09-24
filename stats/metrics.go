package stats

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// Metrics holds Prometheus-style counters. Pure stdlib: exposition is
// hand-written text/plain in the Prometheus format.
type Metrics struct {
	// Global request counters by outcome class.
	ReqTotal      atomic.Uint64
	Req2xx        atomic.Uint64
	Req4xx        atomic.Uint64
	Req5xx        atomic.Uint64
	RateLimited   atomic.Uint64
	QuotaExceeded atomic.Uint64
	Failovers     atomic.Uint64
	Retries       atomic.Uint64
	AdminDenied   atomic.Uint64
	LogDropped    func() uint64

	mu         sync.Mutex
	upstream   map[string]*upCounters
	latencySum atomic.Int64 // ms total for avg
	latencyN   atomic.Int64
}

type upCounters struct {
	Requests  atomic.Uint64
	Errors    atomic.Uint64
	Failovers atomic.Uint64
}

func NewMetrics() *Metrics {
	return &Metrics{upstream: make(map[string]*upCounters)}
}

func (m *Metrics) counters(name string) *upCounters {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.upstream[name]
	if c == nil {
		c = &upCounters{}
		m.upstream[name] = c
	}
	return c
}

func (m *Metrics) ObserveRequest(status int, latencyMS int64) {
	m.ReqTotal.Add(1)
	m.latencySum.Add(latencyMS)
	m.latencyN.Add(1)
	switch {
	case status >= 200 && status < 300:
		m.Req2xx.Add(1)
	case status >= 400 && status < 500:
		m.Req4xx.Add(1)
	default:
		m.Req5xx.Add(1)
	}
}

func (m *Metrics) ObserveUpstream(name string, ok bool) {
	if name == "" {
		return
	}
	c := m.counters(name)
	c.Requests.Add(1)
	if !ok {
		c.Errors.Add(1)
	}
}

func (m *Metrics) ObserveFailover(from, to string) {
	m.Failovers.Add(1)
	if from != "" {
		m.counters(from).Failovers.Add(1)
	}
	_ = to
}

func (m *Metrics) IncRetry()         { m.Retries.Add(1) }
func (m *Metrics) IncRateLimited()   { m.RateLimited.Add(1) }
func (m *Metrics) IncQuotaExceeded() { m.QuotaExceeded.Add(1) }
func (m *Metrics) IncAdminDenied()   { m.AdminDenied.Add(1) }

// AvgLatencyMS returns the running average (0 if no samples).
func (m *Metrics) AvgLatencyMS() int64 {
	n := m.latencyN.Load()
	if n == 0 {
		return 0
	}
	return m.latencySum.Load() / n
}

// WritePrometheus renders the current snapshot as Prometheus text exposition.
// health may be nil.
func (m *Metrics) WritePrometheus(w http.ResponseWriter, health []HealthStatus) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	var b strings.Builder

	writeCounter := func(name, help string, v uint64) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s counter\n%s %d\n", name, help, name, name, v)
	}

	writeCounter("liapi_requests_total", "Total client requests handled.", m.ReqTotal.Load())
	writeCounter("liapi_requests_2xx_total", "Requests that returned 2xx.", m.Req2xx.Load())
	writeCounter("liapi_requests_4xx_total", "Requests that returned 4xx.", m.Req4xx.Load())
	writeCounter("liapi_requests_5xx_total", "Requests that returned 5xx.", m.Req5xx.Load())
	writeCounter("liapi_rate_limited_total", "Requests rejected by the client rate limiter.", m.RateLimited.Load())
	writeCounter("liapi_quota_exceeded_total", "Requests rejected by the daily quota.", m.QuotaExceeded.Load())
	writeCounter("liapi_failovers_total", "Upstream failover events.", m.Failovers.Load())
	writeCounter("liapi_retries_total", "Same-upstream retries.", m.Retries.Load())
	writeCounter("liapi_admin_denied_total", "Rejected admin API attempts.", m.AdminDenied.Load())
	if m.LogDropped != nil {
		writeCounter("liapi_log_dropped_total", "Log entries dropped under backpressure.", m.LogDropped())
	}

	fmt.Fprintf(&b, "# HELP liapi_avg_latency_ms Running average request latency in milliseconds.\n# TYPE liapi_avg_latency_ms gauge\nliapi_avg_latency_ms %d\n", m.AvgLatencyMS())

	m.mu.Lock()
	names := make([]string, 0, len(m.upstream))
	for k := range m.upstream {
		names = append(names, k)
	}
	sort.Strings(names)
	fmt.Fprintf(&b, "# HELP liapi_upstream_requests_total Requests sent per upstream.\n# TYPE liapi_upstream_requests_total counter\n")
	for _, n := range names {
		c := m.upstream[n]
		fmt.Fprintf(&b, "liapi_upstream_requests_total{upstream=%q} %d\n", n, c.Requests.Load())
	}
	fmt.Fprintf(&b, "# HELP liapi_upstream_errors_total Failed attempts per upstream.\n# TYPE liapi_upstream_errors_total counter\n")
	for _, n := range names {
		c := m.upstream[n]
		fmt.Fprintf(&b, "liapi_upstream_errors_total{upstream=%q} %d\n", n, c.Errors.Load())
	}
	fmt.Fprintf(&b, "# HELP liapi_upstream_failovers_total Times traffic failed over from an upstream.\n# TYPE liapi_upstream_failovers_total counter\n")
	for _, n := range names {
		c := m.upstream[n]
		fmt.Fprintf(&b, "liapi_upstream_failovers_total{upstream=%q} %d\n", n, c.Failovers.Load())
	}
	m.mu.Unlock()

	if health != nil {
		fmt.Fprintf(&b, "# HELP liapi_upstream_healthy Upstream health (1=healthy, 0=unhealthy).\n# TYPE liapi_upstream_healthy gauge\n")
		for _, hs := range health {
			v := 0
			if hs.OK {
				v = 1
			}
			fmt.Fprintf(&b, "liapi_upstream_healthy{upstream=%q} %d\n", hs.Name, v)
		}
		fmt.Fprintf(&b, "# HELP liapi_upstream_latency_ms Last probe latency per upstream.\n# TYPE liapi_upstream_latency_ms gauge\n")
		for _, hs := range health {
			fmt.Fprintf(&b, "liapi_upstream_latency_ms{upstream=%q} %d\n", hs.Name, hs.LatencyMS)
		}
	}

	_, _ = w.Write([]byte(b.String()))
}
