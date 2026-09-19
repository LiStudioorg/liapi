package stats

import (
	"encoding/json"
	"os"
	"sync"
	"sync/atomic"
)

// Entry is one request log record (also one JSONL line).
type Entry struct {
	Time      string  `json:"time"`
	Token     string  `json:"token"`
	Path      string  `json:"path"`
	Model     string  `json:"model"`
	Upstream  string  `json:"upstream"`
	Status    int     `json:"status"`
	Stream    bool    `json:"stream"`
	LatencyMS int64   `json:"latency_ms"`
	InTokens  int     `json:"in_tokens"`
	OutTokens int     `json:"out_tokens"`
	Cost      float64 `json:"cost"`
	Error     string  `json:"error,omitempty"`
}

// Ring is a fixed-size circular buffer (array + head index). Not a slice
// that grows — memory is constant.
type Ring struct {
	mu    sync.Mutex
	data  []Entry
	head  int
	count int
}

func NewRing(size int) *Ring {
	if size < 16 {
		size = 16
	}
	return &Ring{data: make([]Entry, size)}
}

func (r *Ring) Add(e Entry) {
	r.mu.Lock()
	r.data[r.head] = e
	r.head = (r.head + 1) % len(r.data)
	if r.count < len(r.data) {
		r.count++
	}
	r.mu.Unlock()
}

// Recent returns the newest n entries, newest first.
func (r *Ring) Recent(n int) []Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n > r.count {
		n = r.count
	}
	if n < 0 {
		n = 0
	}
	out := make([]Entry, n)
	start := (r.head - 1 + len(r.data)) % len(r.data)
	for i := 0; i < n; i++ {
		out[i] = r.data[(start-i+len(r.data)*(i+1))%len(r.data)]
	}
	return out
}

// Logger appends JSONL to a file and keeps an in-memory ring. All writes go
// through a single background goroutine so disk IO never blocks requests;
// the channel drops (and counts) entries when saturated.
type Logger struct {
	file    *os.File
	ring    *Ring
	ch      chan Entry
	dropped atomic.Uint64
}

func NewLogger(path string, ringSize int) (*Logger, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	l := &Logger{file: f, ring: NewRing(ringSize), ch: make(chan Entry, 4096)}
	go l.run()
	return l, nil
}

func (l *Logger) Log(e Entry) {
	select {
	case l.ch <- e:
	default:
		l.dropped.Add(1)
	}
}

func (l *Logger) Dropped() uint64 { return l.dropped.Load() }

func (l *Logger) Recent(n int) []Entry { return l.ring.Recent(n) }

func (l *Logger) run() {
	enc := json.NewEncoder(l.file)
	for e := range l.ch {
		_ = enc.Encode(e)
		l.ring.Add(e)
	}
}

func (l *Logger) Close() error {
	close(l.ch)
	return l.file.Close()
}
