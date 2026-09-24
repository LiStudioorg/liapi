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
		Devices      int     `json:"devices"`
		Models       int     `json:"models"`
		Failovers    uint64  `json:"failovers"`
		CostToday    float64 `json:"cost_today"`
	}{TotalsSnapshot: snap}
	if snap.Requests > 0 {
		resp.SuccessRate = float64(snap.Success) / float64(snap.Requests) * 100
		resp.AvgLatencyMS = snap.LatencyMS / snap.Requests
	}
	resp.DroppedLogs = s.logger.Dropped()
	resp.Upstreams = len(cfg.Upstreams)
	resp.ClientTokens = len(cfg.ClientTokens)
	resp.Devices = len(cfg.Devices)
	resp.Models = modelCount
	resp.Failovers = s.metrics.Failovers.Load()
	resp.CostToday = s.aggregate.CostToday()
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

// ---------------------------------------------------------------------------
// Devices (hashed tokens, per-device RPM/daily limits)
// ---------------------------------------------------------------------------

func deviceView(d config.Device) map[string]any {
	return map[string]any{
		"id":         d.ID,
		"name":       d.Name,
		"rpm":        d.RPM,
		"daily":      d.Daily,
		"disabled":   d.Disabled,
		"created_at": d.CreatedAt,
		"note":       d.Note,
		// Never expose token_hash fully; prefix is enough for correlation.
		"token_hint": hintHash(d.TokenHash),
	}
}

func hintHash(h string) string {
	if len(h) <= 8 {
		return "***"
	}
	return h[:8] + "..."
}

func (s *Server) adminListDevices(w http.ResponseWriter, r *http.Request) {
	cfg := s.holder.Get()
	out := make([]map[string]any, 0, len(cfg.Devices))
	for _, d := range cfg.Devices {
		out = append(out, deviceView(d))
	}
	common.WriteJSON(w, http.StatusOK, out)
}

// adminCreateDevice issues a new device token. The plaintext token is
// returned exactly once in the response; only its SHA-256 hash is stored.
func (s *Server) adminCreateDevice(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		RPM      int    `json:"rpm"`
		Daily    int    `json:"daily"`
		Note     string `json:"note"`
		Disabled bool   `json:"disabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "")
		return
	}
	token := common.RandomToken("sk-")
	id := strings.TrimSpace(req.ID)
	if id == "" {
		id = "dev-" + common.RequestID()[:8]
	}
	next := s.holder.Get().Clone()
	for _, d := range next.Devices {
		if d.ID == id {
			common.WriteError(w, http.StatusConflict, fmt.Sprintf("device %q already exists", id), "invalid_request_error", "")
			return
		}
	}
	dev := config.Device{
		ID:        id,
		Name:      strings.TrimSpace(req.Name),
		TokenHash: common.HashToken(token),
		RPM:       req.RPM,
		Daily:     req.Daily,
		Disabled:  req.Disabled,
		CreatedAt: time.Now().Format(time.RFC3339),
		Note:      strings.TrimSpace(req.Note),
	}
	next.Devices = append(next.Devices, dev)
	if err := next.Validate(); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "")
		return
	}
	if err := s.applyConfig(next); err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error(), "internal_error", "")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"device": deviceView(dev),
		"token":  token, // shown once — client must store it now
	})
}

func (s *Server) adminUpdateDevice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Name     *string `json:"name"`
		RPM      *int    `json:"rpm"`
		Daily    *int    `json:"daily"`
		Note     *string `json:"note"`
		Disabled *bool   `json:"disabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "")
		return
	}
	next := s.holder.Get().Clone()
	idx := -1
	for i := range next.Devices {
		if next.Devices[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		common.WriteError(w, http.StatusNotFound, "device not found", "not_found", "")
		return
	}
	d := &next.Devices[idx]
	if req.Name != nil {
		d.Name = strings.TrimSpace(*req.Name)
	}
	if req.RPM != nil {
		d.RPM = *req.RPM
	}
	if req.Daily != nil {
		d.Daily = *req.Daily
	}
	if req.Note != nil {
		d.Note = strings.TrimSpace(*req.Note)
	}
	if req.Disabled != nil {
		d.Disabled = *req.Disabled
	}
	s.commitConfig(w, next)
}

func (s *Server) adminDeleteDevice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	next := s.holder.Get().Clone()
	found := false
	out := next.Devices[:0]
	for _, d := range next.Devices {
		if d.ID == id {
			found = true
			continue
		}
		out = append(out, d)
	}
	if !found {
		common.WriteError(w, http.StatusNotFound, "device not found", "not_found", "")
		return
	}
	next.Devices = out
	s.commitConfig(w, next)
}

// adminRotateDevice issues a fresh token for an existing device (old token
// stops working immediately). Plaintext returned once.
func (s *Server) adminRotateDevice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	next := s.holder.Get().Clone()
	idx := -1
	for i := range next.Devices {
		if next.Devices[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		common.WriteError(w, http.StatusNotFound, "device not found", "not_found", "")
		return
	}
	token := common.RandomToken("sk-")
	next.Devices[idx].TokenHash = common.HashToken(token)
	if err := s.applyConfig(next); err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error(), "internal_error", "")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"device": deviceView(next.Devices[idx]),
		"token":  token,
	})
}

// ---------------------------------------------------------------------------
// Logs / health / debug
// ---------------------------------------------------------------------------

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
	model, params := s.router.ResolveAlias(req.Model)
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
	result, failure := s.relay.Do(r.Context(), body, "/v1/chat/completions", req.Model, model, candidates, params, newRequestID())
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

// ---------------------------------------------------------------------------
// Config view / replace / export / import
// ---------------------------------------------------------------------------

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
	// Device hashes are already non-reversible; still avoid echoing them.
	for i := range cfg.Devices {
		cfg.Devices[i].TokenHash = hintHash(cfg.Devices[i].TokenHash)
	}
	common.WriteJSON(w, http.StatusOK, cfg)
}

// adminReplaceConfig is the full-config hot reload endpoint (validate → save
// atomically → swap holder). The old config is kept as holder.prev on any
// failure (swap never happens on validation error).
func (s *Server) adminReplaceConfig(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "")
		return
	}
	next, err := config.Import(data)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "")
		return
	}
	resolveMaskedSecrets(s.holder.Get(), next)
	s.commitConfig(w, next)
}

// adminExportConfig returns the FULL config (secrets included) for backup.
func (s *Server) adminExportConfig(w http.ResponseWriter, r *http.Request) {
	data, err := config.Export(s.holder.Get())
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error(), "internal_error", "")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="liapi-config.json"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
	_, _ = w.Write([]byte("\n"))
}

// adminImportConfig accepts either a native config JSON body, or a
// OneAPI/new-api channels export when ?format=oneapi (upstreams merged into
// the current config).
func (s *Server) adminImportConfig(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "")
		return
	}
	format := r.URL.Query().Get("format")
	switch format {
	case "", "liapi":
		next, err := config.Import(data)
		if err != nil {
			common.WriteError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "")
			return
		}
		resolveMaskedSecrets(s.holder.Get(), next)
		s.commitConfig(w, next)
	case "oneapi":
		ups, err := config.FromOneAPI(data)
		if err != nil {
			common.WriteError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "")
			return
		}
		next := s.holder.Get().Clone()
		next.Upstreams = ups
		s.commitConfig(w, next)
	default:
		common.WriteError(w, http.StatusBadRequest, "format must be liapi or oneapi", "invalid_request_error", "")
	}
}

// ---------------------------------------------------------------------------
// Stats aggregation API
// ---------------------------------------------------------------------------

func (s *Server) adminStats(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	snap := s.aggregate.Query(q.Get("from"), q.Get("to"), q.Get("group_by"))
	common.WriteJSON(w, http.StatusOK, snap)
}

func (s *Server) adminStatsExport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	snap := s.aggregate.Query(q.Get("from"), q.Get("to"), q.Get("group_by"))
	switch q.Get("format") {
	case "", "json":
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="liapi-stats.json"`)
		_ = json.NewEncoder(w).Encode(snap)
	default: // csv
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="liapi-stats.csv"`)
		_ = snap.WriteCSV(w)
	}
}

func (s *Server) adminStatsDevices(w http.ResponseWriter, r *http.Request) {
	n := 20
	if v := r.URL.Query().Get("n"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			n = i
		}
	}
	if n > 200 {
		n = 200
	}
	common.WriteJSON(w, http.StatusOK, s.aggregate.TopDevices(n))
}

// handleMetrics serves Prometheus text format.
// Auth precedence:
//  1. metrics_token == "-"  → open (no auth; explicit opt-in for local scrapes)
//  2. metrics_token set    → that token (query ?token=, Bearer, or x-metrics-token)
//  3. otherwise            → admin token (Authorization or x-admin-token)
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	cfg := s.holder.Get()
	switch {
	case cfg.MetricsToken == "-":
		// explicitly public
	case cfg.MetricsToken != "":
		tok := r.URL.Query().Get("token")
		if tok == "" {
			tok = strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		}
		if tok == "" {
			tok = r.Header.Get("x-metrics-token")
		}
		ok := common.TokenEqual(cfg.MetricsToken, tok)
		if !ok && tok != "" {
			// Admin token remains a valid credential for /metrics.
			ok = s.adminAuth.Authenticate(r)
		}
		if !ok {
			s.metrics.IncAdminDenied()
			common.WriteError(w, http.StatusUnauthorized, "invalid metrics token", "invalid_request_error", "invalid_metrics_token")
			return
		}
	default:
		if !s.adminAuth.Authenticate(r) {
			s.metrics.IncAdminDenied()
			common.WriteError(w, http.StatusUnauthorized, "metrics requires metrics_token or admin token", "invalid_request_error", "invalid_metrics_token")
			return
		}
	}
	s.metrics.WritePrometheus(w, s.health.All())
}

// ---------------------------------------------------------------------------
// Shared
// ---------------------------------------------------------------------------

// resolveMaskedSecrets restores any redacted secret in next (values
// containing "...", as produced by MaskToken/hintHash) from the live config.
// This makes round-tripping GET /admin/api/config → edit → POST safe: masked
// api_key/admin_token/client_tokens/device hashes keep their real values.
func resolveMaskedSecrets(old, next *config.Config) {
	if old == nil || next == nil {
		return
	}
	isMasked := func(s string) bool { return strings.Contains(s, "...") }

	if isMasked(next.AdminToken) {
		next.AdminToken = old.AdminToken
	}

	oldUps := make(map[string]string, len(old.Upstreams))
	for _, u := range old.Upstreams {
		oldUps[u.Name] = u.APIKey
	}
	for i := range next.Upstreams {
		if k, ok := oldUps[next.Upstreams[i].Name]; ok && isMasked(next.Upstreams[i].APIKey) {
			next.Upstreams[i].APIKey = k
		}
	}

	if len(next.ClientTokens) > 0 && len(old.ClientTokens) > 0 {
		// Map masked form → live token; replace in place.
		live := make(map[string]string, len(old.ClientTokens))
		for _, t := range old.ClientTokens {
			live[common.MaskToken(t)] = t
		}
		for i, t := range next.ClientTokens {
			if isMasked(t) {
				if real, ok := live[t]; ok {
					next.ClientTokens[i] = real
				}
			}
		}
	}

	oldDev := make(map[string]string, len(old.Devices))
	for _, d := range old.Devices {
		oldDev[d.ID] = d.TokenHash
	}
	for i := range next.Devices {
		if h, ok := oldDev[next.Devices[i].ID]; ok && isMasked(next.Devices[i].TokenHash) {
			next.Devices[i].TokenHash = h
		}
	}
}

// applyConfig validates, saves atomically, and swaps the holder. Returns an
// error WITHOUT swapping when validation/save fails.
func (s *Server) applyConfig(next *config.Config) error {
	if err := next.Validate(); err != nil {
		return err
	}
	// Restore secrets that were masked in the request payload (admin UI
	// round-trip) before touching disk.
	resolveMaskedSecrets(s.holder.Get(), next)
	if err := config.Save(s.configPath, next); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	if err := s.holder.Set(next); err != nil {
		// Validate passed above; Set only fails if something mutated in
		// between — old config stays active (atomic swap never half-applies).
		return err
	}
	s.syncLimiters(next)
	return nil
}

func (s *Server) syncLimiters(next *config.Config) {
	s.limiter.SetLimit(next.RateLimitPerMinute)
	s.adminLimit.SetLimit(next.AdminRatePerMinute)
	s.quota.SetLimit(next.DailyPerToken)
}

// commitConfig validates, saves atomically, swaps the holder, then syncs the
// rate limiters. All admin mutations go through here.
func (s *Server) commitConfig(w http.ResponseWriter, next *config.Config) {
	if err := s.applyConfig(next); err != nil {
		// Distinguish validation (400) from IO (500).
		if strings.Contains(err.Error(), "failed to save") {
			common.WriteError(w, http.StatusInternalServerError, err.Error(), "internal_error", "")
		} else {
			common.WriteError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "")
		}
		return
	}
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
