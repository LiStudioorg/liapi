package server

import (
	"embed"
	"net/http"

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
	health     *stats.Health
	totals     *stats.Totals
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
	return &Server{
		holder:     holder,
		router:     router,
		relay:      rel,
		clientAuth: clientAuth,
		adminAuth:  adminAuth,
		logger:     logger,
		limiter:    limiter,
		health:     health,
		totals:     totals,
		configPath: configPath,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Business API (OpenAI-compatible) — client auth
	mux.HandleFunc("POST /v1/chat/completions", s.handleChatCompletions)
	mux.HandleFunc("POST /v1/completions", s.handleCompletions)
	mux.HandleFunc("POST /v1/embeddings", s.handleEmbeddings)
	mux.HandleFunc("POST /v1/messages", s.handleClaudeMessages)
	mux.HandleFunc("GET /v1/models", s.handleModels)

	// Admin UI + API (admin auth)
	mux.HandleFunc("GET /admin", s.adminUI)
	mux.HandleFunc("GET /admin/", s.adminUI)
	mux.HandleFunc("GET /admin/api/overview", s.requireAdmin(s.adminOverview))
	mux.HandleFunc("GET /admin/api/upstreams", s.requireAdmin(s.adminListUpstreams))
	mux.HandleFunc("POST /admin/api/upstreams", s.requireAdmin(s.adminAddUpstream))
	mux.HandleFunc("PUT /admin/api/upstreams/{name}", s.requireAdmin(s.adminUpdateUpstream))
	mux.HandleFunc("DELETE /admin/api/upstreams/{name}", s.requireAdmin(s.adminDeleteUpstream))
	mux.HandleFunc("GET /admin/api/tokens", s.requireAdmin(s.adminListTokens))
	mux.HandleFunc("POST /admin/api/tokens", s.requireAdmin(s.adminAddToken))
	mux.HandleFunc("POST /admin/api/tokens/delete", s.requireAdmin(s.adminDeleteToken))
	mux.HandleFunc("GET /admin/api/logs", s.requireAdmin(s.adminLogs))
	mux.HandleFunc("GET /admin/api/health", s.requireAdmin(s.adminHealth))
	mux.HandleFunc("POST /admin/api/test", s.requireAdmin(s.adminTest))
	mux.HandleFunc("GET /admin/api/config", s.requireAdmin(s.adminGetConfig))

	return mux
}

func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.adminAuth.Authenticate(r) {
			common.WriteError(w, http.StatusUnauthorized, "invalid or missing admin token", "invalid_request_error", "invalid_admin_key")
			return
		}
		next(w, r)
	}
}
