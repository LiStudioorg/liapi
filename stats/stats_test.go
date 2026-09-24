package stats

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestLimiterSlidingWindow uses a fake clock: a burst of `limit` is allowed,
// the next is denied even across what would be a fixed-window boundary, and
// requests become allowed again once the window slides past the oldest hit.
func TestLimiterSlidingWindow(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 59, 0, time.UTC)
	l := NewLimiter(3)
	defer l.Close()
	l.SetNow(func() time.Time { return now })

	for i := 0; i < 3; i++ {
		if !l.Allow("k") {
			t.Fatalf("hit %d should pass", i)
		}
	}
	if l.Allow("k") {
		t.Fatal("4th hit in window must be denied")
	}
	// Cross a fixed-window boundary (into the next minute) while the old hits
	// are still inside the sliding 60s lookback → still denied (old fixed
	// window would have allowed a double burst here).
	now = now.Add(2 * time.Second) // 12:01:01 — hits at 12:00:59 age=2s < 60s
	if l.Allow("k") {
		t.Fatal("sliding window must still deny within 60s of the hits")
	}
	// Slide past the first hits (age > 60s) → slots free up.
	now = now.Add(61 * time.Second)
	if !l.Allow("k") {
		t.Fatal("after 60s window slides, request must pass")
	}
	// Independent keys don't interfere.
	if !l.Allow("other") {
		t.Fatal("separate key must have its own budget")
	}
	// limit<=0 disables.
	l.SetLimit(0)
	if !l.Allow("k") {
		t.Fatal("limit 0 disables limiting")
	}
	// AllowN with explicit per-device limit.
	l.SetLimit(10)
	if !l.AllowN("d1", 1) {
		t.Fatal("first per-device hit passes")
	}
	if l.AllowN("d1", 1) {
		t.Fatal("second per-device hit denied")
	}
	if !l.AllowN("d1", 0) {
		t.Fatal("AllowN(...,0) falls back to global (limit 10) and passes")
	}
}

func TestRingRecent(t *testing.T) {
	r := NewRing(16)
	for i := 0; i < 20; i++ {
		r.Add(Entry{Model: "m" + itoa(i)})
	}
	recent := r.Recent(3)
	if len(recent) != 3 {
		t.Fatalf("want 3, got %d", len(recent))
	}
	if recent[0].Model != "m19" || recent[2].Model != "m17" {
		t.Fatalf("unexpected order: %+v", recent)
	}
	if got := r.Recent(100); len(got) != 16 {
		t.Fatalf("want 16 (ring size), got %d", len(got))
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

func TestLoggerRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "r.jsonl")
	// Tiny max size forces rotation after the first few entries.
	lg, err := NewLogger(path, 64, 200)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		lg.Log(Entry{Model: "m", RequestID: strings.Repeat("x", 40)})
	}
	// Close drains the queue.
	if err := lg.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("rotated backup missing: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() > 400 {
		t.Fatalf("active file should stay near rotation threshold, got %d", fi.Size())
	}
	// Log after close is a no-op (no panic).
	lg.Log(Entry{Model: "late"})
}

func TestQuotaDaily(t *testing.T) {
	now := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	q := NewQuota(2)
	defer q.Close()
	q.SetNow(func() time.Time { return now })

	if !q.Allow("a") || !q.Allow("a") {
		t.Fatal("first two hits must pass")
	}
	if q.Allow("a") {
		t.Fatal("third hit exceeds daily quota")
	}
	if !q.Allow("b") {
		t.Fatal("quota is per-key")
	}
	// UTC midnight rollover resets the budget.
	now = time.Date(2026, 3, 2, 0, 0, 1, 0, time.UTC)
	if !q.Allow("a") {
		t.Fatal("new UTC day must reset quota")
	}
	// Unlimited.
	q.SetLimit(0)
	if !q.Allow("a") {
		t.Fatal("limit 0 disables quota")
	}
	// Per-device override: AllowN(key, 0) falls back to global (0 = off).
	if !q.AllowN("c", 0) {
		t.Fatal("AllowN(...,0) with global off must pass")
	}
	q.SetLimit(5)
	if !q.AllowN("d", 1) {
		t.Fatal("first per-device hit passes")
	}
	if q.AllowN("d", 1) {
		t.Fatal("second per-device hit denied")
	}
}

func TestAggregateQueryAndPercentiles(t *testing.T) {
	a := NewAggregate()
	base := time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC)
	// Pin the clock so CostToday matches the entry day regardless of CI date.
	now := base
	a.SetNow(func() time.Time { return now })
	// Spread latencies so P50 < P90 < P99 within known buckets.
	lats := []int64{10, 20, 30, 40, 60, 80, 90, 120, 300, 4000}
	for i, lat := range lats {
		a.Add(Entry{
			Time:      base.Add(time.Duration(i) * time.Minute).Format(time.RFC3339),
			Model:     "m1",
			Device:    "devA",
			Token:     "sk-x...yz",
			Status:    200,
			LatencyMS: lat,
			InTokens:  10,
			OutTokens: 5,
			Cost:      0.01,
		})
	}
	a.Add(Entry{
		Time: base.Format(time.RFC3339), Model: "m1", Device: "devA",
		Status: 500, LatencyMS: 5, Error: "boom",
	})

	snap := a.Query("2026-09-23", "2026-09-23", "model")
	if len(snap.Rows) != 1 {
		t.Fatalf("want 1 model row, got %d", len(snap.Rows))
	}
	row := snap.Rows[0]
	if row.Requests != 11 || row.Success != 10 || row.Errors != 1 {
		t.Fatalf("bad counts: %+v", row)
	}
	if !(row.P50MS > 0 && row.P50MS <= row.P90MS && row.P90MS <= row.P99MS) {
		t.Fatalf("percentiles not ordered: p50=%d p90=%d p99=%d", row.P50MS, row.P90MS, row.P99MS)
	}
	if snap.ErrorCodes["500"] != 1 {
		t.Fatalf("error code distribution missing: %v", snap.ErrorCodes)
	}

	dev := a.Query("", "", "device")
	if len(dev.Rows) != 1 || dev.Rows[0].Key != "devA" {
		t.Fatalf("device grouping failed: %+v", dev.Rows)
	}
	day := a.Query("", "", "day")
	if len(day.Rows) != 1 || day.Rows[0].Key != "2026-09-23" {
		t.Fatalf("day grouping failed: %+v", day.Rows)
	}

	// CSV export has a header + one row.
	var sb strings.Builder
	if err := snap.WriteCSV(&sb); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(sb.String()), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "key,") {
		t.Fatalf("csv export wrong: %q", sb.String())
	}

	top := a.TopDevices(5)
	if len(top) != 1 || top[0].Key != "devA" {
		t.Fatalf("top devices: %+v", top)
	}
	if got := a.CostToday(); got < 0.099 {
		t.Fatalf("cost_today: %v", got)
	}
}

func TestMetricsPrometheus(t *testing.T) {
	m := NewMetrics()
	var dropped atomic.Uint64
	dropped.Store(3)
	m.LogDropped = dropped.Load
	m.ObserveRequest(200, 100)
	m.ObserveRequest(429, 5)
	m.ObserveRequest(502, 50)
	m.ObserveUpstream("up-a", true)
	m.ObserveUpstream("up-a", false)
	m.ObserveFailover("up-a", "up-b")
	m.IncRetry()
	m.IncRateLimited()

	// Render via WritePrometheus into a buffer-like ResponseRecorder substitute.
	rec := &fakeRW{hdr: map[string][]string{}}
	m.WritePrometheus(rec, []HealthStatus{{Name: "up-a", OK: true, LatencyMS: 42}})
	body := rec.body.String()
	for _, want := range []string{
		"liapi_requests_total 3",
		"liapi_requests_2xx_total 1",
		"liapi_requests_4xx_total 1",
		"liapi_requests_5xx_total 1",
		`liapi_upstream_requests_total{upstream="up-a"} 2`,
		`liapi_upstream_errors_total{upstream="up-a"} 1`,
		"liapi_failovers_total 1",
		"liapi_retries_total 1",
		"liapi_rate_limited_total 1",
		"liapi_log_dropped_total 3",
		`liapi_upstream_healthy{upstream="up-a"} 1`,
		`liapi_upstream_latency_ms{upstream="up-a"} 42`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics missing %q", want)
		}
	}
}

func TestHealthPersistRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "health.json")
	cfg := ConfigForTest(path)
	h := NewHealth(cfg, nil)
	h.set("up1", &HealthStatus{Name: "up1", OK: false, ConsecutiveFailures: 2, LatencyMS: 7}, 3)
	h.save()

	// Reload into a fresh Health sharing the same state file.
	h2 := NewHealth(cfg, nil)
	got := h2.All()
	if len(got) != 1 || got[0].Name != "up1" || got[0].ConsecutiveFailures != 2 {
		t.Fatalf("persisted state not restored: %+v", got)
	}
	if h2.IsHealthy("up1") {
		// threshold default 3, failures 2 → still healthy (below threshold)
	}
}
