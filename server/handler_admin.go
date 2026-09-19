package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"liapi/common"
	"liapi/config"
	"liapi/relay"
	"liapi/stats"
)

func (s *Server) adminUI(w http.ResponseWriter, r *http.Request) {
	data, err := ui.ReadFile("adminui/index.html")
	if err != nil {
		http.Error(w, "admin ui not embedded", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

func (s *Server) adminOverview(w http.ResponseWriter, r *http.Request) {
	snap := s.totals.Snapshot()
	cfg := s.holder.Get()

	modelCount := 0
	seen := map[string]bool{}
	for _, u := range cfg.Upstreams {
		if !u.Enabled() {
			continue
		}
		for _, m := range u.Models {
			if m != "*" && !seen[m] {
				seen[m] = true
				modelCount++
			}
		}
	}
	modelCount += len(cfg.Aliases)

	resp := struct {
		stats.TotalsSnapshot
		SuccessRate  float64 `json:"success_rate"`
		AvgLatencyMS int64   `json:"avg_latency_ms"`
		DroppedLogs  uint64  `json:"dropped_logs"`
		Upstreams    int     `json:"upstreams"`
		ClientTokens int     `json:"client_tokens"`
		Models       int     `json:"models"`
	}{TotalsSnapshot: snap}
	if snap.Requests > 0 {
		resp.SuccessRate = float64(snap.Success) / float64(snap.Requests) * 100
		resp.AvgLatencyMS = snap.LatencyMS / snap.Requests
	}
	resp.DroppedLogs = s.logger.Dropped()
	resp.Upstreams = len(cfg.Upstreams)
	resp.ClientTokens = len(cfg.ClientTokens)
	resp.Models = modelCount
	common.WriteJSON(w, http.StatusOK, resp)
}

func upstreamView(u config.Upstream) map[string]any {
	key := ""
	if u.APIKey != "" {
		key = common.MaskToken(u.APIKey)
	}
	return map[string]any{
		"name":         u.Name,
		"base_url":     u.BaseURL,
		"api_key":      key,
		"has_api_key":  u.APIKey != "",
		"models":       u.Models,
		"priority":     u.Priority,
		"weight":       u.Weight,
		"disabled":     u.Disabled,
		"health_path":  u.EffectiveHealthPath(),
		"retry":        u.Retry,
		"inject_usage": u.InjectUsage,
	}
}

func (s *Server) adminListUpstreams(w http.ResponseWriter, r *http.Request) {
	cfg := s.holder.Get()
	out := make([]map[string]any, 0, len(cfg.Upstreams))
	for _, u := range cfg.Upstreams {
		out = append(out, upstreamView(u))
	}
	common.WriteJSON(w, http.StatusOK, out)
}

func (s *Server) adminAddUpstream(w http.ResponseWriter, r *http.Request) {
	var u config.Upstream
	if err := decodeJSON(r, &u); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "")
		return
	}
	next := s.holder.Get().Clone()
	for _, e := range next.Upstreams {
		if e.Name == u.Name {
			common.WriteError(w, http.StatusConflict, fmt.Sprintf("upstream %q already exists", u.Name), "invalid_request_error", "")
			return
		}
	}
	next.Upstreams = append(next.Upstreams, u)
	s.commitConfig(w, next)
}

func (s *Server) adminUpdateUpstream(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var u config.Upstream
	if err := decodeJSON(r, &u); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "")
		return
	}
	next := s.holder.Get().Clone()
	idx := -1
	for i := range next.Upstreams {
		if next.Upstreams[i].Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		common.WriteError(w, http.StatusNotFound, "upstream not found", "not_found", "")
		return
	}
	old := next.Upstreams[idx]
	// Keep the old key when empty or unchanged (masked value sent back).
	if u.APIKey == "" || (old.APIKey != "" && u.APIKey == common.MaskToken(old.APIKey)) {
		u.APIKey = old.APIKey
	}
	if strings.TrimSpace(u.Name) == "" {
		u.Name = name
	}
	next.Upstreams[idx] = u
	s.commitConfig(w, next)
}

func (s *Server) adminDeleteUpstream(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	next := s.holder.Get().Clone()
	found := false
	out := next.Upstreams[:0]
	for _, u := range next.Upstreams {
		if u.Name == name {
			found = true
			continue
		}
		out = append(out, u)
	}
	if !found {
		common.WriteError(w, http.StatusNotFound, "upstream not found", "not_found", "")
		return
	}
	next.Upstreams = out
	s.commitConfig(w, next)
}

func (s *Server) adminListTokens(w http.ResponseWriter, r *http.Request) {
	cfg := s.holder.Get()
	out := make([]map[string]any, 0, len(cfg.ClientTokens))
	for _, t := range cfg.ClientTokens {
		out = append(out, map[string]any{
			"masked": common.MaskToken(t),
			"full":   t,
		})
	}
	common.WriteJSON(w, http.StatusOK, out)
}

func (s *Server) adminAddToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "")
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" {
		common.WriteError(w, http.StatusBadRequest, "token is required", "invalid_request_error", "")
		return
	}
	next := s.holder.Get().Clone()
	for _, t := range next.ClientTokens {
		if t == req.Token {
			common.WriteError(w, http.StatusConflict, "token already exists", "invalid_request_error", "")
			return
		}
	}
	next.ClientTokens = append(next.ClientTokens, req.Token)
	s.commitConfig(w, next)
}

func (s *Server) adminDeleteToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "")
		return
	}
	next := s.holder.Get().Clone()
	found := false
	out := next.ClientTokens[:0]
	for _, t := range next.ClientTokens {
		if t == req.Token {
			found = true
			continue
		}
		out = append(out, t)
	}
	if !found {
		common.WriteError(w, http.StatusNotFound, "token not found", "not_found", "")
		return
	}
	next.ClientTokens = out
	s.commitConfig(w, next)
}

func (s *Server) adminLogs(w http.ResponseWriter, r *http.Request) {
	n := 100
	if v := r.URL.Query().Get("n"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			n = i
		}
	}
	if n < 1 {
		n = 1
	}
	if n > 1000 {
		n = 1000
	}
	common.WriteJSON(w, http.StatusOK, s.logger.Recent(n))
}

func (s *Server) adminHealth(w http.ResponseWriter, r *http.Request) {
	common.WriteJSON(w, http.StatusOK, s.health.All())
}

func (s *Server) adminTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model     string `json:"model"`
		Messages  []any  `json:"messages"`
		Stream    bool   `json:"stream"`
		MaxTokens int    `json:"max_tokens"`
	}
	if err := decodeJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "")
		return
	}
	if strings.TrimSpace(req.Model) == "" {
		common.WriteError(w, http.StatusBadRequest, "model is required", "invalid_request_error", "")
		return
	}
	if len(req.Messages) == 0 {
		common.WriteError(w, http.StatusBadRequest, "messages is required", "invalid_request_error", "")
		return
	}

	start := time.Now()
	model := s.router.ResolveModel(req.Model)
	payload := map[string]any{
		"model":    model,
		"messages": req.Messages,
		"stream":   req.Stream,
	}
	if req.MaxTokens > 0 {
		payload["max_tokens"] = req.MaxTokens
	}
	body, err := json.Marshal(payload)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error(), "internal_error", "")
		return
	}
	candidates := s.router.Candidates(model)

	out := map[string]any{"ok": false, "model": model}
	result, failure := s.relay.Do(r.Context(), body, "/v1/chat/completions", model, model, candidates)
	if failure != nil {
		out["upstream"] = failure.Upstream
		if failure.PassResponse != nil {
			data, _ := io.ReadAll(io.LimitReader(failure.PassResponse.Body, 1<<20))
			out["status"] = failure.PassResponse.StatusCode
			out["error"] = string(data)
			failure.Close()
		} else {
			st, msg := relay.ErrStatus(failure)
			out["status"] = st
			out["error"] = msg
		}
	} else {
		data := result.BodyBytes(1 << 20)
		out["ok"] = result.StatusCode() >= 200 && result.StatusCode() < 300
		out["status"] = result.StatusCode()
		out["upstream"] = result.Upstream
		out["body"] = string(data)
		in, outT := relay.ExtractUsage(data)
		out["usage"] = map[string]int{"in_tokens": in, "out_tokens": outT}
	}
	out["latency_ms"] = time.Since(start).Milliseconds()
	common.WriteJSON(w, http.StatusOK, out)
}

func (s *Server) adminGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.holder.Get().Clone()
	for i := range cfg.Upstreams {
		if cfg.Upstreams[i].APIKey != "" {
			cfg.Upstreams[i].APIKey = common.MaskToken(cfg.Upstreams[i].APIKey)
		}
	}
	if cfg.AdminToken != "" {
		cfg.AdminToken = common.MaskToken(cfg.AdminToken)
	}
	for i := range cfg.ClientTokens {
		cfg.ClientTokens[i] = common.MaskToken(cfg.ClientTokens[i])
	}
	common.WriteJSON(w, http.StatusOK, cfg)
}

// commitConfig validates, saves atomically, swaps the holder, then syncs the
// rate limiter. All admin mutations go through here.
func (s *Server) commitConfig(w http.ResponseWriter, next *config.Config) {
	if err := next.Validate(); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "")
		return
	}
	if err := config.Save(s.configPath, next); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to save config: "+err.Error(), "internal_error", "")
		return
	}
	s.holder.Set(next)
	s.limiter.SetLimit(next.RateLimitPerMinute)
	common.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func decodeJSON(r *http.Request, v any) error {
	data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("empty request body")
	}
	if err := json.Unmarshal(data, v); err != nil {
		var syn *json.SyntaxError
		if errors.As(err, &syn) {
			return fmt.Errorf("invalid JSON: syntax error at offset %d", syn.Offset)
		}
		return fmt.Errorf("invalid JSON: %v", err)
	}
	return nil
}
