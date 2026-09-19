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
	"liapi/config"
	"liapi/relay"
	"liapi/routing"
	"liapi/server"
	"liapi/stats"
)

func main() {
	configPath := flag.String("config", "config.json", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if len(cfg.ClientTokens) == 0 {
		log.Printf("[warn] no client_tokens configured yet — all /v1/* requests will be rejected")
	}
	log.Printf("config loaded from %s (admin_token=%s)", *configPath, cfg.AdminToken)

	holder := config.NewHolder(cfg)

	logger, err := stats.NewLogger(cfg.LogFile, cfg.RingSize)
	if err != nil {
		log.Fatalf("open log file %s: %v", cfg.LogFile, err)
	}
	defer logger.Close()

	limiter := stats.NewLimiter(cfg.RateLimitPerMinute)
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

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 15 * time.Second,
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

	log.Printf("liapi listening on %s", cfg.Addr)
	// stdin may be needed by some environments; ensure logs are visible.
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}
