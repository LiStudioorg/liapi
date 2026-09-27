package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"liapi/auth"
	"liapi/common"
	"liapi/config"
	"liapi/relay"
	"liapi/routing"
	"liapi/server"
	"liapi/stats"
)

// version is overridden at link time by buildrelease.sh (-X main.version=…).
var version = "dev"

// printAdminBanner surfaces how to log in to the admin UI. The login account
// is username/password; on first run the generated password is shown once here
// (it is never stored in plaintext). The static admin token is also printed as
// a fallback credential for /metrics and automation.
func printAdminBanner(cfg *config.Config, configPath string) {
	token := cfg.AdminToken
	if os.Getenv("LIAPI_MASK_ADMIN_TOKEN") == "1" {
		token = common.MaskToken(cfg.AdminToken) + "  (masked; see " + configPath + ")"
	}
	pw := "(unchanged — set via 管理台 配置页)"
	if cfg.FirstRunPassword != "" {
		pw = cfg.FirstRunPassword + "  (首次生成，仅显示这一次)"
	} else if os.Getenv("LIAPI_MASK_ADMIN_TOKEN") == "1" {
		pw = "(masked; reset via config)"
	}
	const bar = "════════════════════════════════════════════════════════════════"
	log.Printf("\n%s\n  Liapi 管理台  http://<host>%s/\n\n  登录入口    http://<host>%s/\n  用户名      %s\n  密码        %s\n\n  Admin Token :  %s\n  （Token 仅用于 /metrics 与脚本；日常登录用用户名+密码）\n%s",
		bar, cfg.Addr, cfg.Addr, cfg.AdminUsername, pw, token, bar)
}

func main() {
	configPath := flag.String("config", "config.json", "path to config file")
	showVer := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVer {
		log.Printf("liapi %s", version)
		return
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if len(cfg.ClientTokens) == 0 && len(cfg.Devices) == 0 {
		log.Printf("[warn] no client_tokens/devices configured yet — all /v1/* requests will be rejected")
	}

	holder := config.NewHolder(cfg)

	log.Printf("config loaded from %s", *configPath)
	printAdminBanner(cfg, *configPath)
	cfg.FirstRunPassword = ""

	logger, err := stats.NewLogger(cfg.LogFile, cfg.RingSize, cfg.LogMaxBytes)
	if err != nil {
		log.Fatalf("open log file %s: %v", cfg.LogFile, err)
	}
	defer logger.Close()

	limiter := stats.NewLimiter(cfg.RateLimitPerMinute)
	defer limiter.Close()
	totals := stats.NewTotals()

	rel := relay.New(holder)
	health := stats.NewHealth(holder, rel.Client())
	router := routing.NewRouter(holder, health)
	clientAuth := auth.NewClient(holder)
	adminAuth := auth.NewAdmin(holder)

	srv := server.New(holder, router, rel, clientAuth, adminAuth, logger, limiter, health, totals, *configPath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	health.Start(ctx)

	idle := cfg.StreamIdleTimeout.Duration
	if idle < 0 {
		idle = 0
	}
	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 15 * time.Second,
		// IdleTimeout only applies between requests on a keep-alive conn; it
		// does not cut active streams (those are governed by stream_idle_timeout).
		IdleTimeout: 60 * time.Second,
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-done
		log.Println("shutting down...")
		cancel()
		shutdownCtx, sc := context.WithTimeout(context.Background(), 10*time.Second)
		defer sc()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()

	log.Printf("liapi %s listening on %s (version=%s)", version, cfg.Addr, version)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}
