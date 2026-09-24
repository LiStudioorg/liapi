package stats

import (
	"sync"
	"time"
)

// Limiter enforces a sliding window (one minute) per key. limit <= 0
// disables limiting. Stored hits are pruned on every access and by a
// background sweeper, so memory stays bounded.
type Limiter struct {
	mu     sync.Mutex
	limit  int
	hits   map[string][]time.Time
	nowFn  func() time.Time
	closed chan struct{}
	once   sync.Once
}

// NewLimiter creates a limiter; limit 0 disables. nowFn may be nil (tests
// inject a fake clock).
func NewLimiter(limit int) *Limiter {
	l := &Limiter{
		limit:  limit,
		hits:   make(map[string][]time.Time),
		nowFn:  time.Now,
		closed: make(chan struct{}),
	}
	go l.cleanLoop()
	return l
}

func (l *Limiter) SetNow(fn func() time.Time) {
	l.mu.Lock()
	if fn != nil {
		l.nowFn = fn
	}
	l.mu.Unlock()
}

func (l *Limiter) SetLimit(n int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.limit = n
	if n <= 0 {
		l.hits = make(map[string][]time.Time)
	}
}

func (l *Limiter) Limit() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.limit
}

// Allow reports whether the key may proceed and records the hit.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.allowLocked(key, l.limit)
}

// AllowN checks against an explicit limit (per-device override); n<=0 means
// unlimited. Falls back to the global limit only when n is not provided —
// call Allow for that.
func (l *Limiter) AllowN(key string, n int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if n == 0 {
		// n==0 means "use global policy": unlimited only if global disabled.
		return l.allowLocked(key, l.limit)
	}
	if n < 0 {
		return true
	}
	return l.allowLocked(key, n)
}

func (l *Limiter) allowLocked(key string, limit int) bool {
	if limit <= 0 {
		return true
	}
	now := l.nowFn()
	cutoff := now.Add(-time.Minute)
	hits := l.hits[key]
	// Drop expired prefixes.
	i := 0
	for i < len(hits) && !hits[i].After(cutoff) {
		i++
	}
	hits = hits[i:]
	if len(hits) >= limit {
		l.hits[key] = hits
		return false
	}
	l.hits[key] = append(hits, now)
	return true
}

// Remaining reports how many requests are left in the current window (for a
// global or override limit). Used by tests/metrics, not the hot path.
func (l *Limiter) Remaining(key string, limit int) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if limit <= 0 {
		return -1
	}
	now := l.nowFn()
	cutoff := now.Add(-time.Minute)
	n := 0
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			n++
		}
	}
	if n >= limit {
		return 0
	}
	return limit - n
}

func (l *Limiter) cleanLoop() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-l.closed:
			return
		case <-t.C:
			l.mu.Lock()
			now := l.nowFn()
			cutoff := now.Add(-time.Minute)
			for k, hits := range l.hits {
				i := 0
				for i < len(hits) && !hits[i].After(cutoff) {
					i++
				}
				if i == 0 {
					continue
				}
				if i >= len(hits) {
					delete(l.hits, k)
				} else {
					l.hits[k] = hits[i:]
				}
			}
			l.mu.Unlock()
		}
	}
}

// Close stops the background sweeper (tests).
func (l *Limiter) Close() {
	l.once.Do(func() { close(l.closed) })
}
