package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"liapi/config"
)

// Result carries a successful upstream response together with its context
// cancellation. Close() must be called when done reading the body.
type Result struct {
	resp     *http.Response
	cancel   context.CancelFunc
	Upstream string
	Model    string
	Stream   bool
}

func (r *Result) StatusCode() int { return r.resp.StatusCode }

func (r *Result) Close() {
	if r.cancel != nil {
		r.cancel()
	}
	if r.resp.Body != nil {
		r.resp.Body.Close()
	}
}

// WriteTo copies the upstream response to the client, then releases
// resources. Returns (stream, inTokens, outTokens, error).
func (r *Result) WriteTo(w http.ResponseWriter) (bool, int, int, error) {
	defer r.Close()
	return CopyResponse(w, r.resp)
}

// BodyBytes reads the upstream body fully (bounded by limit) and releases
// resources. Used by the admin test endpoint.
func (r *Result) BodyBytes(limit int64) []byte {
	defer r.Close()
	data, _ := io.ReadAll(io.LimitReader(r.resp.Body, limit))
	return data
}

// Failure describes why an attempt (or all attempts) failed.
type Failure struct {
	Upstream     string
	Status       int
	Snippet      string
	Message      string
	PassResponse *http.Response // set for 4xx(≠429): pass upstream error through
	cancel       context.CancelFunc
}

func (f *Failure) Error() string {
	if f.Message != "" {
		return f.Message
	}
	if f.Snippet != "" {
		return fmt.Sprintf("upstream %q returned HTTP %d: %s", f.Upstream, f.Status, compactSnippet(f.Snippet))
	}
	return fmt.Sprintf("upstream %q error (HTTP %d)", f.Upstream, f.Status)
}

// PassTo copies a PassResponse to the client untouched, then releases.
func (f *Failure) PassTo(w http.ResponseWriter) (bool, int, int, error) {
	defer f.Close()
	return CopyResponse(w, f.PassResponse)
}

func (f *Failure) Close() {
	if f.cancel != nil {
		f.cancel()
	}
	if f.PassResponse != nil && f.PassResponse.Body != nil {
		f.PassResponse.Body.Close()
	}
}

func compactSnippet(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		s = s[:500] + "..."
	}
	return s
}

// ErrStatus maps a final Failure (after all retries/failover exhausted) to an
// HTTP status + message suitable for the client.
func ErrStatus(f *Failure) (int, string) {
	if f.Status == http.StatusTooManyRequests {
		return http.StatusTooManyRequests, "all upstreams are rate limited (429): " + f.Error()
	}
	if f.Status >= 500 && f.Status > 0 {
		return http.StatusBadGateway, f.Error()
	}
	if f.Status >= 400 && f.Status != 0 {
		return f.Status, f.Error()
	}
	return http.StatusBadGateway, f.Error()
}

type Relay struct {
	holder *config.Holder
	client *http.Client
}

func New(holder *config.Holder) *Relay {
	cfg := holder.Get()
	tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          cfg.MaxIdleConns,
		MaxIdleConnsPerHost:   cfg.MaxIdleConns,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &Relay{holder: holder, client: &http.Client{Transport: tr}}
}

// Client exposes the shared http.Client (so health checks reuse connections).
func (rl *Relay) Client() *http.Client { return rl.client }

// DetectStream reports whether the request body requests streaming.
func DetectStream(body []byte) bool {
	var req struct {
		Stream *bool `json:"stream"`
	}
	if json.Unmarshal(body, &req) != nil {
		return false
	}
	return req.Stream != nil && *req.Stream
}

// Do forwards the request to candidates in order with failover:
//   - 4xx (except 429) -> pass upstream's error through, stop immediately.
//   - connection error / timeout / 429 / 5xx -> try next retry/upstream.
func (rl *Relay) Do(ctx context.Context, body []byte, clientPath, originalModel, model string, candidates []config.Upstream) (*Result, *Failure) {
	if len(candidates) == 0 {
		return nil, &Failure{Message: fmt.Sprintf("no upstream configured for model %q", model)}
	}
	if err := ctx.Err(); err != nil {
		return nil, &Failure{Message: "client cancelled"}
	}

	relPath := relativePath(clientPath)
	stream := DetectStream(body)
	var last *Failure

	for i := range candidates {
		up := &candidates[i]
		attempts := up.Retry + 1
		for a := 1; a <= attempts; a++ {
			if a > 1 {
				select {
				case <-ctx.Done():
					return nil, &Failure{Upstream: up.Name, Message: "client cancelled"}
				case <-time.After(250 * time.Millisecond * time.Duration(a)):
				}
			}
			res, f := rl.tryOnce(ctx, up, relPath, body, originalModel, model, stream)
			if f == nil {
				return res, nil
			}
			last = f
			if f.PassResponse != nil {
				return nil, f
			}
		}
	}
	return nil, last
}

// tryOnce performs a single attempt against one upstream.
func (rl *Relay) tryOnce(ctx context.Context, up *config.Upstream, relPath string, body []byte, originalModel, model string, stream bool) (*Result, *Failure) {
	cfg := rl.holder.Get()

	reqBody := prepareBody(body, originalModel, model, stream, up.InjectUsage)

	target := up.BaseURL + relPath

	var attemptCtx context.Context = ctx
	var cancel context.CancelFunc
	if stream {
		if cfg.StreamTimeout.Duration > 0 {
			attemptCtx, cancel = context.WithTimeout(ctx, cfg.StreamTimeout.Duration)
		}
	} else if cfg.Timeout.Duration > 0 {
		attemptCtx, cancel = context.WithTimeout(ctx, cfg.Timeout.Duration)
	}

	req, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, target, bytes.NewReader(reqBody))
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, &Failure{Upstream: up.Name, Message: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	switch {
	case up.APIKey == "":
		// local upstream without auth (e.g. Ollama)
	case relPath == "/messages":
		req.Header.Set("x-api-key", up.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	default:
		req.Header.Set("Authorization", "Bearer "+up.APIKey)
	}

	resp, err := rl.client.Do(req)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, &Failure{Upstream: up.Name, Message: err.Error()}
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return &Result{resp: resp, cancel: cancel, Upstream: up.Name, Model: model, Stream: stream}, nil
	}

	// 4xx (except 429): the request itself is at fault — pass upstream error
	// through verbatim, do not fail over.
	if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
		return nil, &Failure{Upstream: up.Name, Status: resp.StatusCode, PassResponse: resp, cancel: cancel}
	}

	// 429 / 5xx / other: read a small snippet for logging, discard, retry.
	snippet := string(readAtMost(resp.Body, 8192))
	resp.Body.Close()
	if cancel != nil {
		cancel()
	}
	return nil, &Failure{Upstream: up.Name, Status: resp.StatusCode, Snippet: snippet}
}

// prepareBody rewrites the outbound payload only when needed:
//   - the resolved model differs from the request's model field (aliases), and/or
//   - stream_options.include_usage should be injected.
//
// If nothing needs to change, the original bytes are forwarded untouched.
func prepareBody(body []byte, originalModel, resolvedModel string, stream, injectUsage bool) []byte {
	needModel := originalModel != "" && originalModel != resolvedModel
	if !needModel && !(stream && injectUsage) {
		return body
	}
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		return body
	}
	if needModel {
		m["model"] = resolvedModel
	}
	if stream && injectUsage {
		opts, _ := m["stream_options"].(map[string]any)
		if opts == nil {
			opts = map[string]any{}
			m["stream_options"] = opts
		}
		opts["include_usage"] = true
	}
	if data, err := json.Marshal(m); err == nil {
		return data
	}
	return body
}

// relativePath strips the /v1 prefix from a client path so it can be joined
// onto base_url (which already carries the version prefix).
func relativePath(clientPath string) string {
	if clientPath == "/v1" || clientPath == "" {
		return "/"
	}
	if strings.HasPrefix(clientPath, "/v1") {
		return strings.TrimPrefix(clientPath, "/v1")
	}
	return clientPath
}

func readAtMost(r io.Reader, n int64) []byte {
	data, _ := io.ReadAll(io.LimitReader(r, n))
	return data
}
