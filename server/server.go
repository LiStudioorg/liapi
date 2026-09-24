package server

import (
	"embed"
	"net"
	"net/http"
	"strings"

	"liapi/auth"
	"liapi/common"
	"liapi/config"
	"liapi/relay"
	"liapi/routing"
	"liapi/stats"
)

//go:embed adminui/index.html
var ui embed.FS

type Server struct {
	holder     *config.Holder
	router     *routing.Router
	relay      *relay.Relay
	clientAuth *auth.Client
	adminAuth  *auth.Admin
	logger     *stats.Logger
	limiter    *stats.Limiter
	adminLimit *stats.Limiter
	quota      *stats.Quota
	health     *stats.Health
	totals     *stats.Totals
	metrics    *stats.Metrics
	aggregate  *stats.Aggregate
	configPath string
}

func New(
	holder *config.Holder,
	router *routing.Router,
	rel *relay.Relay,
	clientAuth *auth.Client,
	adminAuth *auth.Admin,
	logger *stats.Logger,
	limiter *stats.Limiter,
	health *stats.Health,
	totals *stats.Totals,
	configPath string,
) *Server {
	cfg := holder.Get()
	metrics := stats.NewMetrics()
	metrics.LogDropped = logger.Dropped
	agg := stats.NewAggregate()
	quota := stats.NewQuota(cfg.DailyPerToken)
	adminLimit := stats.NewLimiter(cfg.AdminRatePerMinute)
	// Wire relay → metrics (retry/failover events).
	rel.SetObserver(func(kind, from, to string) {
		if kind == "failover" {
			metrics.ObserveFailover(from, to)
		} else {
			metrics.IncRetry()
		}
	})
	return &Server{
		holder:     holder,
		router:     router,
		relay:      rel,
		clientAuth: clientAuth,
		adminAuth:  adminAuth,
		logger:     logger,
		limiter:    limiter,
		adminLimit: adminLimit,
		quota:      quota,
		health:     health,
		totals:     totals,
		metrics:    metrics,
		aggregate:  agg,
		configPath: configPath,
	}
}

// Metrics exposes the metrics registry (wired by main for /metrics).
func (s *Server) Metrics() *stats.Metrics { return s.metrics }

// Aggregate exposes the usage aggregate (admin stats API).
func (s *Server) Aggregate() *stats.Aggregate { return s.aggregate }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Business API (OpenAI-compatible) — client auth
	mux.HandleFunc("POST /v1/chat/completions", s.handleChatCompletions)
	mux.HandleFunc("POST /v1/completions", s.handleCompletions)
	mux.HandleFunc("POST /v1/embeddings", s.handleEmbeddings)
	mux.HandleFunc("POST /v1/messages", s.handleClaudeMessages)
	mux.HandleFunc("GET /v1/models", s.handleModels)

	// Observability (metrics token OR admin token)
	mux.HandleFunc("GET /metrics", s.handleMetrics)

	// Admin UI at / (primary); /admin kept as redirect for old bookmarks.
	mux.HandleFunc("GET /{$}", s.adminUI)
	mux.HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /admin/", func(w http.ResponseWriter, r *http.Request) {
		// Only exact /admin/ (no API path) → redirect; API falls through below.
		if r.URL.Path == "/admin/" {
			http.Redirect(w, r, "/", http.StatusMovedPermanently)
			return
		}
		s.adminUI(w, r)
	})
	mux.HandleFunc("GET /admin/api/overview", s.requireAdmin(s.adminOverview))
	mux.HandleFunc("GET /admin/api/upstreams", s.requireAdmin(s.adminListUpstreams))
	mux.HandleFunc("POST /admin/api/upstreams", s.requireAdmin(s.adminAddUpstream))
	mux.HandleFunc("PUT /admin/api/upstreams/{name}", s.requireAdmin(s.adminUpdateUpstream))
	mux.HandleFunc("DELETE /admin/api/upstreams/{name}", s.requireAdmin(s.adminDeleteUpstream))
	mux.HandleFunc("GET /admin/api/tokens", s.requireAdmin(s.adminListTokens))
	mux.HandleFunc("POST /admin/api/tokens", s.requireAdmin(s.adminAddToken))
	mux.HandleFunc("POST /admin/api/tokens/delete", s.requireAdmin(s.adminDeleteToken))
	mux.HandleFunc("GET /admin/api/devices", s.requireAdmin(s.adminListDevices))
	mux.HandleFunc("POST /admin/api/devices", s.requireAdmin(s.adminCreateDevice))
	mux.HandleFunc("PUT /admin/api/devices/{id}", s.requireAdmin(s.adminUpdateDevice))
	mux.HandleFunc("DELETE /admin/api/devices/{id}", s.requireAdmin(s.adminDeleteDevice))
	mux.HandleFunc("POST /admin/api/devices/{id}/rotate", s.requireAdmin(s.adminRotateDevice))
	mux.HandleFunc("GET /admin/api/logs", s.requireAdmin(s.adminLogs))
	mux.HandleFunc("GET /admin/api/health", s.requireAdmin(s.adminHealth))
	mux.HandleFunc("POST /admin/api/test", s.requireAdmin(s.adminTest))
	mux.HandleFunc("GET /admin/api/config", s.requireAdmin(s.adminGetConfig))
	mux.HandleFunc("POST /admin/api/config", s.requireAdmin(s.adminReplaceConfig))
	mux.HandleFunc("GET /admin/api/config/export", s.requireAdmin(s.adminExportConfig))
	mux.HandleFunc("POST /admin/api/config/import", s.requireAdmin(s.adminImportConfig))
	mux.HandleFunc("GET /admin/api/stats", s.requireAdmin(s.adminStats))
	mux.HandleFunc("GET /admin/api/stats/export", s.requireAdmin(s.adminStatsExport))
	mux.HandleFunc("GET /admin/api/stats/devices", s.requireAdmin(s.adminStatsDevices))

	// Request-ID middleware: echo X-Request-ID (generate when absent) on
	// every response so clients/logs/upstream headers correlate.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, withRequestID(w, r))
	})
}

// clientIP extracts the peer IP, honoring X-Forwarded-For only when the
// config opts in (trust_proxy via admin_allow_ips + explicit header use).
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Only honor XFF when an allowlist is configured (deployment behind
		// a trusted proxy); otherwise spoofable.
		if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
			return first
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func ipAllowed(ip string, allow []string) bool {
	if len(allow) == 0 {
		return true
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, a := range allow {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if strings.Contains(a, "/") {
			if _, n, err := net.ParseCIDR(a); err == nil && n.Contains(parsed) {
				return true
			}
			continue
		}
		if v := net.ParseIP(a); v != nil && v.Equal(parsed) {
			return true
		}
	}
	return false
}

// requireAdmin enforces: IP allowlist → admin rate limit → admin token.
func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := s.holder.Get()
		ip := clientIP(r)
		if !ipAllowed(ip, cfg.AdminAllowIPs) {
			s.metrics.IncAdminDenied()
			common.WriteError(w, http.StatusForbidden, "ip not allowed", "permission_error", "ip_forbidden")
			return
		}
		// Rate limit keyed by IP (protects the token from brute force).
		if !s.adminLimit.Allow("ip:" + ip) {
			s.metrics.IncAdminDenied()
			common.WriteError(w, http.StatusTooManyRequests, "admin rate limit exceeded", "rate_limit_error", "rate_limit_exceeded")
			return
		}
		if !s.adminAuth.Authenticate(r) {
			s.metrics.IncAdminDenied()
			common.WriteError(w, http.StatusUnauthorized, "invalid or missing admin token", "invalid_request_error", "invalid_admin_key")
			return
		}
		next(w, r)
	}
}
