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
	// Never log raw secrets: mask the admin token.
	log.Printf("config loaded from %s (admin_token=%s)", *configPath, common.MaskToken(cfg.AdminToken))

	holder := config.NewHolder(cfg)

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
