package stats

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Entry is one request log record (also one JSONL line).
type Entry struct {
	Time       string  `json:"time"`
	RequestID  string  `json:"request_id,omitempty"`
	Token      string  `json:"token"`
	Device     string  `json:"device,omitempty"`
	Path       string  `json:"path"`
	Model      string  `json:"model"`
	Upstream   string  `json:"upstream"`
	Status     int     `json:"status"`
	Stream     bool    `json:"stream"`
	LatencyMS  int64   `json:"latency_ms"`
	InTokens   int     `json:"in_tokens"`
	OutTokens  int     `json:"out_tokens"`
	Multimodal int     `json:"multimodal,omitempty"` // image/audio parts in the request body
	Cost       float64 `json:"cost"`
	Error      string  `json:"error,omitempty"`
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
	mu            sync.Mutex
	file          *os.File
	path          string
	maxSize       int64
	written       int64
	ring          *Ring
	ch            chan Entry
	done          chan struct{}
	dropped       atomic.Uint64
	closed        atomic.Bool
	curDay        string // YYYY-MM-DD of the active file (daily rotation)
	rawTok        bool   // when true, tokens are written unmasked to disk
	retentionDays int
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
		curDay:  time.Now().UTC().Format("2006-01-02"),
	}
	if fi, err := f.Stat(); err == nil {
		l.written = fi.Size()
	}
	go l.run()
	return l, nil
}

// SetRawTokens toggles writing the full token value into JSONL lines
// (default: masked). Applies to entries queued after the call.
func (l *Logger) SetRawTokens(on bool) {
	l.mu.Lock()
	l.rawTok = on
	l.mu.Unlock()
}

// RawTokens reports whether the JSONL log stores tokens unmasked.
func (l *Logger) RawTokens() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rawTok
}

// SetRetentionDays sets how many rotated daily backups (path.YYYY-MM-DD)
// to keep; 0 disables purging.
func (l *Logger) SetRetentionDays(n int) {
	l.mu.Lock()
	l.retentionDays = n
	l.mu.Unlock()
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
	purge := time.NewTicker(time.Hour)
	defer purge.Stop()
	for {
		select {
		case e, ok := <-l.ch:
			if !ok {
				return
			}
			l.writeEntry(e)
		case <-purge.C:
			l.purgeOld()
		}
	}
}

func (l *Logger) writeEntry(e Entry) {
	line, err := json.Marshal(e)
	if err != nil {
		return
	}
	line = append(line, '\n')
	l.mu.Lock()
	// Daily rollover: start a fresh file when the UTC day changes.
	day := time.Now().UTC().Format("2006-01-02")
	if day != l.curDay {
		l.rotateDailyLocked(l.curDay)
		l.curDay = day
	}
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

// rotateDailyLocked rolls the active file to <path>.YYYY-MM-DD, keeping the
// previous days around for retention-based purging.
func (l *Logger) rotateDailyLocked(prevDay string) {
	if prevDay == "" {
		return
	}
	_ = l.file.Close()
	backup := l.path + "." + prevDay
	_ = os.Remove(backup)
	if err := os.Rename(l.path, backup); err != nil {
		_ = os.Truncate(l.path, 0)
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		f, _ = os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	}
	l.file = f
	l.written = 0
	l.purgeOldLocked()
}

// purgeOld removes rotated daily backups older than retentionDays.
func (l *Logger) purgeOld() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.purgeOldLocked()
}

func (l *Logger) purgeOldLocked() {
	if l.retentionDays <= 0 {
		return
	}
	dir := filepath.Dir(l.path)
	base := filepath.Base(l.path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -l.retentionDays).Format("2006-01-02")
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if !strings.HasPrefix(name, base+".") {
			continue
		}
		day := strings.TrimPrefix(name, base+".")
		if len(day) != 10 {
			continue
		}
		if _, err := time.Parse("2006-01-02", day); err != nil {
			continue
		}
		if day < cutoff {
			_ = os.Remove(filepath.Join(dir, name))
		}
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
