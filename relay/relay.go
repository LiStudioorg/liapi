package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"liapi/config"
)

// Result carries a successful upstream response together with its context
// cancellation. Close() must be called when done reading the body.
type Result struct {
	resp      *http.Response
	cancel    context.CancelFunc
	Upstream  string
	Model     string
	Stream    bool
	IdleAfter time.Duration
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
	return CopyResponse(w, r.resp, r.IdleAfter)
}

// BodyBytes reads the upstream body fully (bounded by limit) and releases
// resources. Used by the admin test endpoint.
func (r *Result) BodyBytes(limit int64) []byte {
	defer r.Close()
	data, _ := io.ReadAll(io.LimitReader(r.resp.Body, limit))
	return data
}

// failKind classifies an attempt failure so the retry/failover policy can
// decide what is safe to redo.
type failKind int

const (
	failNone     failKind = iota
	failNetwork           // conn refused/reset/DNS — request likely never processed
	failTimeout           // deadline exceeded — request MAY have been processed
	failUpstream          // 429 / 5xx — upstream answered, may or may not have processed
)

// Failure describes why an attempt (or all attempts) failed.
type Failure struct {
	Upstream     string
	Status       int
	Snippet      string
	Message      string
	PassResponse *http.Response // set for 4xx(≠429): pass upstream error through
	Kind         failKind
	Cancelled    bool // client disconnected — never retry
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
	return CopyResponse(w, f.PassResponse, 0)
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

// ErrStatus maps a final Failure (after all retries/failovers exhausted) to an
// HTTP status + message suitable for the client.
func ErrStatus(f *Failure) (int, string) {
	if f.Cancelled {
		return 499, "client closed request"
	}
	if f.Kind == failTimeout {
		return http.StatusGatewayTimeout, "upstream timeout: " + f.Error()
	}
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

// OnEvent is an optional observer for retries/failovers (metrics).
type OnEvent func(kind, from, to string)

type Relay struct {
	holder  *config.Holder
	client  *http.Client
	observe OnEvent
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

// SetObserver registers a callback for retry/failover events (metrics).
func (rl *Relay) SetObserver(fn OnEvent) { rl.observe = fn }

func (rl *Relay) emit(kind, from, to string) {
	if rl.observe != nil {
		rl.observe(kind, from, to)
	}
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

// canRetrySame reports whether re-sending to the SAME upstream is considered
// safe given the failure kind. Policy:
//   - client cancelled / 4xx pass-through: never
//   - network (never left the client): always safe
//   - timeout: only when retry_on_timeout (request may have been processed
//     → re-sending a non-idempotent POST could double-bill)
//   - 429/5xx: yes — upstream rejected/failed, standard failover practice;
//     the caller's upstream-level Retry budget bounds how often.
func (rl *Relay) canRetrySame(f *Failure) bool {
	if f == nil || f.Cancelled || f.PassResponse != nil {
		return false
	}
	switch f.Kind {
	case failNetwork, failUpstream:
		return true
	case failTimeout:
		return rl.holder.Get().RetryOnTimeout
	default:
		return false
	}
}

// canFailover reports whether trying the NEXT upstream is allowed.
// Timeouts obey retry_on_timeout; everything else matches canRetrySame.
func (rl *Relay) canFailover(f *Failure) bool {
	return rl.canRetrySame(f)
}

// Do forwards the request to candidates in order with failover:
//   - 4xx (except 429) -> pass upstream's error through, stop immediately.
//   - network error    -> retry same, then failover (always safe).
//   - timeout          -> only when retry_on_timeout (avoids double-billing).
//   - 429 / 5xx        -> retry same (bounded by upstream.retry), then failover.
func (rl *Relay) Do(ctx context.Context, body []byte, clientPath, originalModel, model string, candidates []config.Upstream, params map[string]any, requestID string) (*Result, *Failure) {
	if len(candidates) == 0 {
		return nil, &Failure{Message: fmt.Sprintf("no upstream configured for model %q", model)}
	}
	if err := ctx.Err(); err != nil {
		return nil, &Failure{Message: "client cancelled", Cancelled: true}
	}

	relPath := relativePath(clientPath)
	stream := DetectStream(body)
	var last *Failure

	for i := range candidates {
		up := &candidates[i]
		attempts := up.Retry + 1
		for a := 1; a <= attempts; a++ {
			if a > 1 {
				if !rl.canRetrySame(last) {
					break
				}
				select {
				case <-ctx.Done():
					return nil, &Failure{Upstream: up.Name, Message: "client cancelled", Cancelled: true}
				case <-time.After(250 * time.Millisecond * time.Duration(a)):
				}
				rl.emit("retry", up.Name, up.Name)
			}
			res, f := rl.tryOnce(ctx, up, relPath, body, originalModel, model, stream, params, requestID)
			if f == nil {
				return res, nil
			}
			last = f
			if f.Cancelled {
				return nil, f
			}
			if f.PassResponse != nil {
				return nil, f
			}
			if !rl.canRetrySame(f) {
				break
			}
		}
		// Decide failover to the next candidate.
		if last == nil || !rl.canFailover(last) {
			return nil, last
		}
		if i+1 < len(candidates) {
			rl.emit("failover", up.Name, candidates[i+1].Name)
		}
	}
	return nil, last
}

// tryOnce performs a single attempt against one upstream.
func (rl *Relay) tryOnce(ctx context.Context, up *config.Upstream, relPath string, body []byte, originalModel, model string, stream bool, params map[string]any, requestID string) (*Result, *Failure) {
	cfg := rl.holder.Get()

	// Per-upstream model mapping (channel-level rename) wins over the
	// alias-resolved name for this hop only.
	finalModel := model
	if to, ok := up.ModelMap[model]; ok && to != "" {
		finalModel = to
	}

	reqBody := prepareBody(body, originalModel, finalModel, stream, up.InjectUsage, params)

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
		return nil, &Failure{Upstream: up.Name, Message: err.Error(), Kind: failNetwork}
	}
	req.Header.Set("Content-Type", "application/json")
	if requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
	}
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
		return nil, rl.classifyDoError(ctx, attemptCtx, up.Name, err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return &Result{resp: resp, cancel: cancel, Upstream: up.Name, Model: model, Stream: stream, IdleAfter: cfg.StreamIdleTimeout.Duration}, nil
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
	return nil, &Failure{Upstream: up.Name, Status: resp.StatusCode, Snippet: snippet, Kind: failUpstream}
}

// classifyDoError distinguishes client-cancel / timeout / pure network errors.
func (rl *Relay) classifyDoError(parent, attempt context.Context, upName string, err error) *Failure {
	f := &Failure{Upstream: upName, Message: err.Error()}
	if parent.Err() != nil {
		f.Cancelled = true
		return f
	}
	if errors.Is(err, context.DeadlineExceeded) || isNetTimeout(err) {
		f.Kind = failTimeout
		return f
	}
	f.Kind = failNetwork
	return f
}

func isNetTimeout(err error) bool {
	var ne net.Error
	if errors.As(err, &ne) {
		return ne.Timeout()
	}
	return false
}

// prepareBody rewrites the outbound payload only when needed:
//   - the resolved model differs from the request's model field (aliases), and/or
//   - stream_options.include_usage should be injected, and/or
//   - alias-rule parameter overrides are present.
//
// If nothing needs to change, the original bytes are forwarded untouched.
func prepareBody(body []byte, originalModel, resolvedModel string, stream, injectUsage bool, params map[string]any) []byte {
	needModel := originalModel != "" && originalModel != resolvedModel
	needParams := len(params) > 0
	if !needModel && !needParams && !(stream && injectUsage) {
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
	// Parameter overrides are applied last so they always win.
	for k, v := range params {
		m[k] = v
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
