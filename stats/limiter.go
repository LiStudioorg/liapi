package stats

import (
	"sync"
	"time"
)

type bucket struct {
	count int
	reset time.Time
}

// Limiter enforces a fixed window: per client token, per minute.
// limit == 0 disables limiting. Expired buckets are purged regularly.
type Limiter struct {
	mu      sync.Mutex
	limit   int
	buckets map[string]*bucket
}

func NewLimiter(limit int) *Limiter {
	l := &Limiter{limit: limit, buckets: make(map[string]*bucket)}
	go l.cleanLoop()
	return l
}

func (l *Limiter) SetLimit(n int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.limit = n
	if n <= 0 {
		l.buckets = make(map[string]*bucket)
	}
}

// Allow reports whether the token may proceed and increments its counter.
func (l *Limiter) Allow(token string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.limit <= 0 {
		return true
	}
	now := time.Now()
	b, ok := l.buckets[token]
	if !ok || b.reset.Before(now) {
		b = &bucket{count: 0, reset: now.Add(time.Minute)}
		l.buckets[token] = b
	}
	if b.count >= l.limit {
		return false
	}
	b.count++
	return true
}

func (l *Limiter) cleanLoop() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for range t.C {
		now := time.Now()
		l.mu.Lock()
		for k, b := range l.buckets {
			if b.reset.Before(now) {
				delete(l.buckets, k)
			}
		}
		l.mu.Unlock()
	}
}
