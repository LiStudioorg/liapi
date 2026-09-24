package relay

import (
	"bufio"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// copyable headers are passed through; hop-by-hop headers are dropped.
// Content-Length must not be passed through: non-stream it is recomputed by
// the server, streaming it is unknown.
func copyHeaders(w http.ResponseWriter, hdr http.Header) {
	for k, vv := range hdr {
		lk := strings.ToLower(k)
		switch lk {
		case "content-length", "transfer-encoding", "connection", "keep-alive",
			"proxy-authenticate", "proxy-authorization", "te", "trailer", "upgrade":
			continue
		}
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
}

// CopyResponse writes an upstream response to the client, extracting usage.
// idleAfter > 0 arms an inactivity watchdog for streaming responses.
// Returns (stream, inTokens, outTokens, error).
func CopyResponse(w http.ResponseWriter, resp *http.Response, idleAfter time.Duration) (bool, int, int, error) {
	copyHeaders(w, resp.Header)
	if isStreaming(resp) {
		in, out, err := copyStream(w, resp, idleAfter)
		return true, in, out, err
	}
	in, out, err := copyPlain(w, resp)
	return false, in, out, err
}

func isStreaming(resp *http.Response) bool {
	return strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")
}

// copyPlain copies a non-stream body. While writing it keeps a sliding
// 64KB tail buffer so usage can be extracted without buffering the whole body.
func copyPlain(w http.ResponseWriter, resp *http.Response) (int, int, error) {
	w.WriteHeader(resp.StatusCode)
	tail := &TailBuffer{max: 64 << 10}
	buf := make([]byte, 32<<10)
	_, err := io.CopyBuffer(io.MultiWriter(w, tail), resp.Body, buf)
	resp.Body.Close()
	in, out := ExtractUsage(tail.Bytes())
	return in, out, err
}

// copyStream relays an SSE stream line-by-line, flushing after each write.
//
// Failure handling:
//   - client write error → stop reading upstream at once.
//   - idle watchdog (idleAfter > 0): if no line arrives for idleAfter, close
//     the upstream body (unblocks Read) and set a past write deadline on the
//     client (unblocks a stalled Write) so a hung peer cannot pin the
//     connection forever.
func copyStream(w http.ResponseWriter, resp *http.Response, idleAfter time.Duration) (int, int, error) {
	w.WriteHeader(resp.StatusCode)
	flusher, _ := w.(http.Flusher)
	rc := http.NewResponseController(w)

	var lastActivity atomic.Int64
	lastActivity.Store(time.Now().UnixNano())

	var watchdogStop chan struct{}
	if idleAfter > 0 {
		watchdogStop = make(chan struct{})
		go func() {
			interval := idleAfter / 4
			if interval > 5*time.Second {
				interval = 5 * time.Second
			}
			if interval < 100*time.Millisecond {
				interval = 100 * time.Millisecond
			}
			t := time.NewTicker(interval)
			defer t.Stop()
			for {
				select {
				case <-watchdogStop:
					return
				case <-t.C:
					if time.Since(time.Unix(0, lastActivity.Load())) >= idleAfter {
						_ = resp.Body.Close()
						_ = rc.SetWriteDeadline(time.Now())
						return
					}
				}
			}
		}()
		defer func() {
			close(watchdogStop)
		}()
		// Refresh the write deadline on every successful write so a live
		// stream is never cut; only a fully idle one expires.
		_ = rc.SetWriteDeadline(time.Now().Add(idleAfter))
	}

	br := bufio.NewReaderSize(resp.Body, 64<<10)
	var in, out int
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			lastActivity.Store(time.Now().UnixNano())
			if _, werr := w.Write(line); werr != nil {
				resp.Body.Close()
				return in, out, werr
			}
			if flusher != nil {
				flusher.Flush()
			}
			if idleAfter > 0 {
				_ = rc.SetWriteDeadline(time.Now().Add(idleAfter))
			}
			lastActivity.Store(time.Now().UnixNano())
			if data := sseData([]byte(strings.TrimSuffix(string(line), "\n"))); len(data) > 0 {
				i, o := ExtractUsage(data)
				if i > in {
					in = i
				}
				if o > out {
					out = o
				}
			}
		}
		if err != nil {
			break
		}
	}
	resp.Body.Close()
	return in, out, nil
}

// sseData extracts the payload of a "data: ..." SSE line, or nil for
// comments/heartbeats/[DONE].
func sseData(line []byte) []byte {
	s := strings.TrimSpace(string(line))
	if !strings.HasPrefix(s, "data:") {
		return nil
	}
	s = strings.TrimSpace(strings.TrimPrefix(s, "data:"))
	if s == "" || s == "[DONE]" {
		return nil
	}
	return []byte(s)
}
