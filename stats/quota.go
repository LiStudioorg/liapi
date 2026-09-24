package stats

import (
	"sync"
	"time"
)

// Quota enforces a per-key daily request cap (UTC day). limit <= 0 disables.
// Memory is bounded by a sweeper that drops keys whose day has rolled over
// and that were not touched again.
type Quota struct {
	mu     sync.Mutex
	limit  int
	used   map[string]*dayCount
	nowFn  func() time.Time
	closed chan struct{}
	once   sync.Once

	// OnWarn, when set, fires once per key per day as usage crosses
	// warnPercent of the effective limit (never when limit <= 0).
	OnWarn      func(key string, used, limit, percent int)
	warnPercent int
	warned      map[string]string // key -> day already warned
}

type dayCount struct {
	day   string // YYYY-MM-DD (UTC)
	count int
}

func NewQuota(limit int) *Quota {
	q := &Quota{
		limit:       limit,
		used:        make(map[string]*dayCount),
		warned:      make(map[string]string),
		warnPercent: 80,
		nowFn:       time.Now,
		closed:      make(chan struct{}),
	}
	go q.cleanLoop()
	return q
}

// SetWarnPercent configures the usage warning threshold (0 disables).
func (q *Quota) SetWarnPercent(p int) {
	q.mu.Lock()
	q.warnPercent = p
	q.mu.Unlock()
}

func (q *Quota) SetNow(fn func() time.Time) {
	q.mu.Lock()
	if fn != nil {
		q.nowFn = fn
	}
	q.mu.Unlock()
}

func (q *Quota) SetLimit(n int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.limit = n
	if n <= 0 {
		q.used = make(map[string]*dayCount)
	}
}

// Allow consumes one unit against key's daily budget.
func (q *Quota) Allow(key string) bool { return q.AllowN(key, q.limit) }

// AllowN checks against an explicit per-device limit (0 = fall back to
// global, which itself may be 0 = unlimited).
func (q *Quota) AllowN(key string, limit int) bool {
	if limit == 0 {
		limit = q.global()
	}
	if limit <= 0 {
		return true
	}
	q.mu.Lock()
	day := q.nowFn().UTC().Format("2006-01-02")
	c := q.used[key]
	if c == nil || c.day != day {
		c = &dayCount{day: day}
		q.used[key] = c
	}
	if c.count >= limit {
		q.mu.Unlock()
		return false
	}
	c.count++
	shouldWarn := false
	used := c.count
	if q.OnWarn != nil && q.warnPercent > 0 && q.warned[key] != day {
		pct := used * 100 / limit
		if pct >= q.warnPercent {
			q.warned[key] = day
			shouldWarn = true
		}
	}
	cb, lim, pct := q.OnWarn, limit, q.warnPercent
	q.mu.Unlock()
	if shouldWarn && cb != nil {
		cb(key, used, lim, pct)
	}
	return true
}

func (q *Quota) global() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.limit
}

// Remaining returns left for today under the given limit (-1 = unlimited).
func (q *Quota) Remaining(key string, limit int) int {
	if limit == 0 {
		limit = q.global()
	}
	if limit <= 0 {
		return -1
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	day := q.nowFn().UTC().Format("2006-01-02")
	c := q.used[key]
	if c == nil || c.day != day {
		return limit
	}
	if c.count >= limit {
		return 0
	}
	return limit - c.count
}

func (q *Quota) cleanLoop() {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-q.closed:
			return
		case <-t.C:
			q.mu.Lock()
			day := q.nowFn().UTC().Format("2006-01-02")
			for k, c := range q.used {
				if c.day != day && c.count > 0 {
					// Keep entry but zero it lazily via day check on next Allow;
					// delete fully stale keys to bound memory.
					if c.day < day {
						delete(q.used, k)
					}
				}
			}
			q.mu.Unlock()
		}
	}
}

// Close stops the sweeper (tests).
func (q *Quota) Close() {
	q.once.Do(func() { close(q.closed) })
}
