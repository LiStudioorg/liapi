package stats

import (
	"encoding/csv"
	"io"
	"sort"
	"strconv"
	"sync"
	"time"
)

// Aggregate accumulates usage by UTC day / device / model / status with
// latency histograms for P50/P90/P99. In-memory only (JSONL remains the
// durable audit trail).
type Aggregate struct {
	mu        sync.Mutex
	byDay     map[string]*bucketGroup // YYYY-MM-DD
	byDev     map[string]*oneStat
	byModel   map[string]*oneStat
	byStat    map[int]int64 // status code -> count
	total     *oneStat
	costByDay map[string]float64
	nowFn     func() time.Time
}

// Hist is a fixed-bucket latency histogram (milliseconds).
type Hist struct {
	bounds  []float64
	counts  []uint64
	sum     float64
	samples uint64
}

var defaultBounds = []float64{50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000, 60000}

func NewHist() *Hist {
	return &Hist{bounds: defaultBounds, counts: make([]uint64, len(defaultBounds)+1)}
}

func (h *Hist) Observe(ms float64) {
	h.sum += ms
	h.samples++
	for i, b := range h.bounds {
		if ms <= b {
			h.counts[i]++
			return
		}
	}
	h.counts[len(h.bounds)]++
}

// Quantile returns the approximate p-quantile (0..1) in ms (upper bound of
// the containing bucket).
func (h *Hist) Quantile(q float64) float64 {
	if h.samples == 0 {
		return 0
	}
	if q <= 0 {
		q = 0.01
	}
	if q >= 1 {
		q = 0.999
	}
	target := uint64(float64(h.samples)*q + 0.5)
	if target == 0 {
		target = 1
	}
	var cum uint64
	for i, c := range h.counts {
		cum += c
		if cum >= target {
			if i < len(h.bounds) {
				return h.bounds[i]
			}
			return h.bounds[len(h.bounds)-1] * 2
		}
	}
	return h.bounds[len(h.bounds)-1] * 2
}

type oneStat struct {
	Requests  int64
	Success   int64
	Errors    int64
	InTokens  int64
	OutTokens int64
	Cost      float64
	LatencyMS int64 // sum, for avg
	Hist      *Hist
}

func newOneStat() *oneStat { return &oneStat{Hist: NewHist()} }

func (s *oneStat) add(e Entry) {
	s.Requests++
	switch {
	case e.Status >= 200 && e.Status < 300:
		s.Success++
	case e.Status >= 400:
		s.Errors++
	}
	s.InTokens += int64(e.InTokens)
	s.OutTokens += int64(e.OutTokens)
	s.Cost += e.Cost
	s.LatencyMS += e.LatencyMS
	if s.Hist == nil {
		s.Hist = NewHist()
	}
	s.Hist.Observe(float64(e.LatencyMS))
}

// bucketGroup is a per-day rollup that also keeps model/device breakdowns.
type bucketGroup struct {
	oneStat
	byModel map[string]*oneStat
	byDev   map[string]*oneStat
}

func (g *bucketGroup) model(m string) *oneStat {
	s := g.byModel[m]
	if s == nil {
		s = newOneStat()
		g.byModel[m] = s
	}
	return s
}

func (g *bucketGroup) dev(d string) *oneStat {
	s := g.byDev[d]
	if s == nil {
		s = newOneStat()
		g.byDev[d] = s
	}
	return s
}

// StatRow is one aggregated row returned by the stats API / export.
type StatRow struct {
	Key      string  `json:"key"`
	Requests int64   `json:"requests"`
	Success  int64   `json:"success"`
	Errors   int64   `json:"errors"`
	InTok    int64   `json:"in_tokens"`
	OutTok   int64   `json:"out_tokens"`
	Cost     float64 `json:"cost"`
	AvgMS    int64   `json:"avg_latency_ms"`
	P50MS    int64   `json:"p50_ms"`
	P90MS    int64   `json:"p90_ms"`
	P99MS    int64   `json:"p99_ms"`
}

// Snapshot is the full stats API payload.
type Snapshot struct {
	From       string           `json:"from"`
	To         string           `json:"to"`
	GroupBy    string           `json:"group_by"`
	Total      StatRow          `json:"total"`
	Rows       []StatRow        `json:"rows"`
	ErrorCodes map[string]int64 `json:"error_codes"`
}

func NewAggregate() *Aggregate {
	return &Aggregate{
		byDay:     make(map[string]*bucketGroup),
		byDev:     make(map[string]*oneStat),
		byModel:   make(map[string]*oneStat),
		byStat:    make(map[int]int64),
		total:     newOneStat(),
		costByDay: make(map[string]float64),
		nowFn:     time.Now,
	}
}

// SetNow injects a fake clock (tests); nil keeps the current function.
func (a *Aggregate) SetNow(fn func() time.Time) {
	a.mu.Lock()
	if fn != nil {
		a.nowFn = fn
	}
	a.mu.Unlock()
}

func (a *Aggregate) model(m string) *oneStat {
	s := a.byModel[m]
	if s == nil {
		s = newOneStat()
		a.byModel[m] = s
	}
	return s
}

func (a *Aggregate) dev(d string) *oneStat {
	s := a.byDev[d]
	if s == nil {
		s = newOneStat()
		a.byDev[d] = s
	}
	return s
}

func modelKey(m string) string {
	if m == "" {
		return "(none)"
	}
	return m
}

func (a *Aggregate) Add(e Entry) {
	day := e.Time
	if t, err := time.Parse(time.RFC3339, e.Time); err == nil {
		day = t.UTC().Format("2006-01-02")
	} else if len(day) >= 10 {
		day = day[:10]
	} else {
		day = time.Now().UTC().Format("2006-01-02")
	}

	dev := e.Device
	if dev == "" {
		dev = e.Token
	}
	if dev == "" {
		dev = "(unknown)"
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	g := a.byDay[day]
	if g == nil {
		g = &bucketGroup{byModel: make(map[string]*oneStat), byDev: make(map[string]*oneStat)}
		a.byDay[day] = g
	}
	g.add(e)
	g.model(modelKey(e.Model)).add(e)
	g.dev(dev).add(e)

	a.model(modelKey(e.Model)).add(e)
	a.dev(dev).add(e)
	a.total.add(e)
	a.costByDay[day] += e.Cost

	if e.Status >= 400 {
		a.byStat[e.Status]++
	}
}

// Query returns an aggregated snapshot.
// groupBy: day | model | device (default model).
// from/to: YYYY-MM-DD inclusive (empty = all retained).
func (a *Aggregate) Query(from, to, groupBy string) Snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()

	if groupBy == "" {
		groupBy = "model"
	}
	agg := make(map[string]*oneStat)
	var days []string
	for d := range a.byDay {
		if from != "" && d < from {
			continue
		}
		if to != "" && d > to {
			continue
		}
		days = append(days, d)
	}
	sort.Strings(days)

	for _, d := range days {
		g := a.byDay[d]
		switch groupBy {
		case "day":
			s := agg[d]
			if s == nil {
				s = newOneStat()
				agg[d] = s
			}
			mergeStat(s, &g.oneStat)
		case "device":
			for k, v := range g.byDev {
				s := agg[k]
				if s == nil {
					s = newOneStat()
					agg[k] = s
				}
				mergeStat(s, v)
			}
		default: // model
			for k, v := range g.byModel {
				s := agg[k]
				if s == nil {
					s = newOneStat()
					agg[k] = s
				}
				mergeStat(s, v)
			}
		}
	}

	snap := Snapshot{From: from, To: to, GroupBy: groupBy, ErrorCodes: make(map[string]int64)}
	for k, v := range agg {
		snap.Rows = append(snap.Rows, rowOf(k, v))
	}
	sort.Slice(snap.Rows, func(i, j int) bool { return snap.Rows[i].Requests > snap.Rows[j].Requests })

	tot := newOneStat()
	for _, d := range days {
		mergeStat(tot, &a.byDay[d].oneStat)
	}
	snap.Total = rowOf("total", tot)

	for code, n := range a.byStat {
		snap.ErrorCodes[strconv.Itoa(code)] = n
	}
	return snap
}

func mergeStat(dst, src *oneStat) {
	dst.Requests += src.Requests
	dst.Success += src.Success
	dst.Errors += src.Errors
	dst.InTokens += src.InTokens
	dst.OutTokens += src.OutTokens
	dst.Cost += src.Cost
	dst.LatencyMS += src.LatencyMS
	if dst.Hist == nil {
		dst.Hist = NewHist()
	}
	if src.Hist != nil {
		if len(dst.Hist.counts) != len(src.Hist.counts) {
			return
		}
		for i, c := range src.Hist.counts {
			dst.Hist.counts[i] += c
		}
		dst.Hist.sum += src.Hist.sum
		dst.Hist.samples += src.Hist.samples
	}
}

func rowOf(key string, s *oneStat) StatRow {
	r := StatRow{
		Key: key, Requests: s.Requests, Success: s.Success, Errors: s.Errors,
		InTok: s.InTokens, OutTok: s.OutTokens, Cost: s.Cost,
	}
	if s.Requests > 0 {
		r.AvgMS = s.LatencyMS / s.Requests
	}
	if s.Hist != nil {
		r.P50MS = int64(s.Hist.Quantile(0.50))
		r.P90MS = int64(s.Hist.Quantile(0.90))
		r.P99MS = int64(s.Hist.Quantile(0.99))
	}
	return r
}

// WriteCSV streams the snapshot as CSV.
func (snap Snapshot) WriteCSV(w io.Writer) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()
	head := []string{"key", "requests", "success", "errors", "in_tokens", "out_tokens", "cost", "avg_latency_ms", "p50_ms", "p90_ms", "p99_ms"}
	if err := cw.Write(head); err != nil {
		return err
	}
	for _, r := range snap.Rows {
		rec := []string{
			r.Key,
			strconv.FormatInt(r.Requests, 10),
			strconv.FormatInt(r.Success, 10),
			strconv.FormatInt(r.Errors, 10),
			strconv.FormatInt(r.InTok, 10),
			strconv.FormatInt(r.OutTok, 10),
			strconv.FormatFloat(r.Cost, 'f', 6, 64),
			strconv.FormatInt(r.AvgMS, 10),
			strconv.FormatInt(r.P50MS, 10),
			strconv.FormatInt(r.P90MS, 10),
			strconv.FormatInt(r.P99MS, 10),
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	return nil
}

// TopDevices returns the top-N devices by request count (usage leaderboard).
func (a *Aggregate) TopDevices(n int) []StatRow {
	a.mu.Lock()
	defer a.mu.Unlock()
	rows := make([]StatRow, 0, len(a.byDev))
	for k, v := range a.byDev {
		rows = append(rows, rowOf(k, v))
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Requests > rows[j].Requests })
	if n > 0 && len(rows) > n {
		rows = rows[:n]
	}
	return rows
}

// CostToday returns today's estimated cost (yuan), using the (possibly
// injected) clock for "today".
func (a *Aggregate) CostToday() float64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	day := a.nowFn().UTC().Format("2006-01-02")
	return a.costByDay[day]
}
