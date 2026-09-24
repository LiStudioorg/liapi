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
	RequestID string  `json:"request_id,omitempty"`
	Token     string  `json:"token"`
	Device    string  `json:"device,omitempty"`
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
//
// Size-based rotation: when the active file exceeds maxSize bytes it is
// renamed to <path>.1 (replacing any previous backup) and a fresh file is
// started. maxSize <= 0 disables rotation (SetDefaults always sets one).
type Logger struct {
	mu      sync.Mutex
	file    *os.File
	path    string
	maxSize int64
	written int64
	ring    *Ring
	ch      chan Entry
	done    chan struct{}
	dropped atomic.Uint64
	closed  atomic.Bool
}

func NewLogger(path string, ringSize int, maxSize int64) (*Logger, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	l := &Logger{
		file:    f,
		path:    path,
		maxSize: maxSize,
		ring:    NewRing(ringSize),
		ch:      make(chan Entry, 4096),
		done:    make(chan struct{}),
	}
	if fi, err := f.Stat(); err == nil {
		l.written = fi.Size()
	}
	go l.run()
	return l, nil
}

func (l *Logger) Log(e Entry) {
	if l.closed.Load() {
		return
	}
	select {
	case l.ch <- e:
	default:
		l.dropped.Add(1)
	}
}

func (l *Logger) Dropped() uint64 { return l.dropped.Load() }

func (l *Logger) Recent(n int) []Entry { return l.ring.Recent(n) }

func (l *Logger) run() {
	defer close(l.done)
	for e := range l.ch {
		line, err := json.Marshal(e)
		if err != nil {
			continue
		}
		line = append(line, '\n')
		l.mu.Lock()
		if l.maxSize > 0 && l.written+int64(len(line)) > l.maxSize && l.written > 0 {
			l.rotateLocked()
		}
		n, werr := l.file.Write(line)
		l.written += int64(n)
		if werr != nil {
			// Reopen once; if it still fails the entry is dropped (ring keeps it).
			_ = l.file.Close()
			l.file, _ = os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		}
		l.mu.Unlock()
		l.ring.Add(e)
	}
}

// rotateLocked renames the active file to path.1 and starts fresh.
func (l *Logger) rotateLocked() {
	_ = l.file.Close()
	backup := l.path + ".1"
	_ = os.Remove(backup)
	if err := os.Rename(l.path, backup); err != nil {
		// Rename failed (e.g. cross-device): truncate instead so growth stops.
		_ = os.Truncate(l.path, 0)
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		// Last resort: keep an empty file handle so writes fail cleanly until reopen.
		f, _ = os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	}
	l.file = f
	l.written = 0
}

// Close flushes the queue then closes the file.
func (l *Logger) Close() error {
	if l.closed.Swap(true) {
		return nil
	}
	close(l.ch)
	<-l.done
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// Stats returns (path, current size) for admin display.
func (l *Logger) Stats() (string, int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.path, l.written
}
