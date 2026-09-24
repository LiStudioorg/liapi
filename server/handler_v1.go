package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"liapi/common"
	"liapi/config"
	"liapi/relay"
	"liapi/stats"
)

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	s.handleModelEndpoint(w, r, "/v1/chat/completions")
}

func (s *Server) handleCompletions(w http.ResponseWriter, r *http.Request) {
	s.handleModelEndpoint(w, r, "/v1/completions")
}

func (s *Server) handleEmbeddings(w http.ResponseWriter, r *http.Request) {
	s.handleModelEndpoint(w, r, "/v1/embeddings")
}

func (s *Server) handleClaudeMessages(w http.ResponseWriter, r *http.Request) {
	s.handleModelEndpoint(w, r, "/v1/messages")
}

// handleModelEndpoint is the shared pipeline for JSON-model endpoints:
// auth -> rate limit -> daily quota -> body/model parse -> route -> forward
// (failover) -> copy response -> log.
func (s *Server) handleModelEndpoint(w http.ResponseWriter, r *http.Request, clientPath string) {
	start := time.Now()
	reqID := requestIDFrom(r)
	if reqID == "" {
		reqID = common.RequestID()
	}
	w.Header().Set("X-Request-ID", reqID)

	var (
		status    int
		upstream  string
		inTok     int
		outTok    int
		streamed  bool
		errMsg    string
		logModel  string
		logToken  string
		logDevice string
	)

	defer func() {
		s.record(r, reqID, logToken, logDevice, logModel, upstream, status, streamed, start, inTok, outTok, errMsg)
	}()

	// 1. Auth (device hash first, then legacy plaintext list).
	ident := s.clientAuth.Identify(r)
	if ident == nil {
		common.WriteError(w, http.StatusUnauthorized, "invalid or missing API key", "invalid_request_error", "invalid_api_key")
		status = http.StatusUnauthorized
		return
	}
	logToken = ident.Mask()
	if ident.Device != nil {
		logDevice = ident.Device.ID
	}

	cfg := s.holder.Get()

	// 2. Rate limit (per-device RPM override, else global sliding window).
	limit := cfg.RateLimitPerMinute
	if ident.Device != nil && ident.Device.RPM > 0 {
		limit = ident.Device.RPM
	}
	if !s.limiter.AllowN(ident.Token, limit) {
		s.metrics.IncRateLimited()
		common.WriteError(w, http.StatusTooManyRequests, "rate limit exceeded, try again later", "rate_limit_error", "rate_limit_exceeded")
		status = http.StatusTooManyRequests
		return
	}

	// 2b. Daily quota (per-device daily override, else global).
	quotaLimit := cfg.DailyPerToken
	if ident.Device != nil && ident.Device.Daily > 0 {
		quotaLimit = ident.Device.Daily
	}
	quotaKey := logToken
	if ident.Device != nil {
		quotaKey = "dev:" + ident.DeviceID
	}
	if !s.quota.AllowN(quotaKey, quotaLimit) {
		s.metrics.IncQuotaExceeded()
		common.WriteError(w, http.StatusTooManyRequests, "daily quota exceeded, try again tomorrow (UTC)", "rate_limit_error", "daily_quota_exceeded")
		status = http.StatusTooManyRequests
		return
	}

	// 3. Body + model parse
	body, err := readBody(w, r, cfg.BodyLimitBytes)
	if err != nil {
		status = http.StatusBadRequest
		if errors.Is(err, errBodyTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		return
	}
	model, _, err := parseModel(body)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "")
		status = http.StatusBadRequest
		return
	}
	originalModel := model
	model, params := s.router.ResolveAlias(model)
	logModel = model

	// 4. Route
	candidates := s.router.Candidates(model)
	if len(candidates) == 0 {
		msg := fmt.Sprintf("no upstream configured for model %q", model)
		common.WriteError(w, http.StatusBadGateway, msg, "invalid_request_error", "model_not_found")
		status = http.StatusBadGateway
		errMsg = msg
		return
	}

	// 5. Forward with failover
	result, failure := s.relay.Do(r.Context(), body, clientPath, originalModel, model, candidates, params, reqID)
	if failure != nil {
		if failure.PassResponse != nil {
			// 4xx (≠429) from upstream: pass its error through verbatim.
			upstream = failure.Upstream
			streamed, inTok, outTok, _ = failure.PassTo(w)
			status = failure.PassResponse.StatusCode
			errMsg = fmt.Sprintf("upstream %q returned %d", failure.Upstream, status)
			s.metrics.ObserveUpstream(upstream, false)
			return
		}
		st, msg := relay.ErrStatus(failure)
		upstream = failure.Upstream
		errMsg = msg
		common.WriteError(w, st, msg, "upstream_error", "")
		status = st
		if upstream != "" {
			s.metrics.ObserveUpstream(upstream, false)
		}
		return
	}
	defer result.Close()

	// 6. Copy response back (stream or plain) and extract usage
	streamed, inTok, outTok, _ = result.WriteTo(w)
	status = result.StatusCode()
	upstream = result.Upstream
	s.metrics.ObserveUpstream(upstream, status >= 200 && status < 300)
}

// record writes one log entry (JSONL + ring + totals + aggregate + metrics).
func (s *Server) record(r *http.Request, reqID, logToken, device, model, upstream string, status int, stream bool, start time.Time, in, out int, errMsg string) {
	cfg := s.holder.Get()
	e := stats.Entry{
		Time:      time.Now().Format(time.RFC3339),
		RequestID: reqID,
		Token:     logToken,
		Device:    device,
		Path:      r.URL.Path,
		Model:     model,
		Upstream:  upstream,
		Status:    status,
		Stream:    stream,
		LatencyMS: time.Since(start).Milliseconds(),
		InTokens:  in,
		OutTokens: out,
		Cost:      costFor(model, in, out, cfg.Prices),
		Error:     errMsg,
	}
	s.totals.Add(e)
	s.logger.Log(e)
	s.aggregate.Add(e)
	s.metrics.ObserveRequest(status, e.LatencyMS)
}

// costFor estimates cost in yuan: in/1e6*priceIn + out/1e6*priceOut.
func costFor(model string, in, out int, prices map[string]config.Price) float64 {
	p, ok := prices[model]
	if !ok {
		return 0
	}
	return float64(in)/1e6*p.Input + float64(out)/1e6*p.Output
}

var errBodyTooLarge = errors.New("body too large")

func readBody(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			common.WriteError(w, http.StatusRequestEntityTooLarge, "request body exceeds size limit", "invalid_request_error", "body_too_large")
			return nil, errBodyTooLarge
		}
		common.WriteError(w, http.StatusBadRequest, "failed to read request body", "invalid_request_error", "")
		return nil, err
	}
	if len(data) == 0 {
		common.WriteError(w, http.StatusBadRequest, "empty request body", "invalid_request_error", "")
		return nil, errors.New("empty body")
	}
	return data, nil
}

func parseModel(body []byte) (string, bool, error) {
	var req struct {
		Model  *string `json:"model"`
		Stream *bool   `json:"stream"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		var syn *json.SyntaxError
		if errors.As(err, &syn) {
			return "", false, fmt.Errorf("invalid JSON: syntax error at offset %d", syn.Offset)
		}
		var typ *json.UnmarshalTypeError
		if errors.As(err, &typ) {
			return "", false, fmt.Errorf("invalid JSON: field %q has wrong type", typ.Field)
		}
		return "", false, fmt.Errorf("invalid JSON: %v", err)
	}
	if req.Model == nil || strings.TrimSpace(*req.Model) == "" {
		return "", false, errors.New("missing required field \"model\"")
	}
	return *req.Model, req.Stream != nil && *req.Stream, nil
}

// handleModels lists available models (config + aliases), OpenAI format.
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	token := s.clientAuth.Authenticate(r)
	if token == "" || !s.clientAuth.Valid(token) {
		common.WriteError(w, http.StatusUnauthorized, "invalid or missing API key", "invalid_request_error", "invalid_api_key")
		return
	}

	cfg := s.holder.Get()
	set := map[string]bool{}
	for _, u := range cfg.Upstreams {
		if !u.Enabled() {
			continue
		}
		for _, m := range u.Models {
			if m != "*" {
				set[m] = true
			}
		}
	}
	for a := range cfg.Aliases {
		set[a] = true
	}
	for _, rule := range cfg.AliasRules {
		if !rule.Regex && rule.Pattern != "" {
			set[rule.Pattern] = true
		}
	}

	names := make([]string, 0, len(set))
	for m := range set {
		names = append(names, m)
	}
	sort.Strings(names)

	data := make([]map[string]any, 0, len(names))
	for _, m := range names {
		data = append(data, map[string]any{
			"id":       m,
			"object":   "model",
			"created":  time.Now().Unix(),
			"owned_by": "liapi",
		})
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}
