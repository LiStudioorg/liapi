package relay

import (
	"bufio"
	"io"
	"net/http"
	"strings"
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
// Returns (stream, inTokens, outTokens, error).
func CopyResponse(w http.ResponseWriter, resp *http.Response) (bool, int, int, error) {
	copyHeaders(w, resp.Header)
	if isStreaming(resp) {
		in, out, err := copyStream(w, resp)
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
// If the client disconnects (write error) we stop reading upstream at once.
func copyStream(w http.ResponseWriter, resp *http.Response) (int, int, error) {
	w.WriteHeader(resp.StatusCode)
	flusher, _ := w.(http.Flusher)

	br := bufio.NewReaderSize(resp.Body, 64<<10)
	var in, out int
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			if _, werr := w.Write(line); werr != nil {
				resp.Body.Close()
				return in, out, werr
			}
			if flusher != nil {
				flusher.Flush()
			}
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
	d := strings.TrimSpace(strings.TrimPrefix(s, "data:"))
	if d == "" || d == "[DONE]" {
		return nil
	}
	return []byte(d)
}
