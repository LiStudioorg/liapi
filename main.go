package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
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

// printAdminBanner surfaces how to reach the admin UI. No credentials are
// ever generated: fresh installs ship with login disabled until the user
// sets login_enabled + a password themselves. The static admin token is
// printed as the default way into the console (masked with
// LIAPI_MASK_ADMIN_TOKEN=1).
func printAdminBanner(cfg *config.Config, configPath string) {
	addr := cfg.ListenAddr()
	display := addr
	if h, p := cfg.HostPort(); h == "" || h == "0.0.0.0" || h == "::" {
		display = "localhost:" + itoa(p)
	}
	token := cfg.AdminToken
	if os.Getenv("LIAPI_MASK_ADMIN_TOKEN") == "1" {
		token = common.MaskToken(cfg.AdminToken) + "  (已脱敏；完整值见 " + configPath + ")"
	}
	const bar = "════════════════════════════════════════════════════════════════"
	lines := []string{
		"  管理台        http://" + display + "/",
		"  Admin Token :  " + token,
		"  （用 Token 登录管理台：打开页面后直接输入 Token；日常自动化脚本也可用）",
		"  配置文件      " + configPath,
	}
	if !cfg.LoginAllowed() {
		lines = append(lines,
			"",
			"  用户名/密码登录：未启用。",
			"  想用自己写的账号密码登录，编辑配置文件：",
			`    "login_enabled": true,`,
			`    "admin_username": "你的用户名",`,
			`    "admin_password": "你的密码"      ← 保存后自动转为哈希，明文不再保留`,
		)
	} else {
		lines = append(lines, "  登录账号      "+cfg.AdminUsername+" （用户名+密码登录已启用）")
	}
	log.Printf("\n%s\n%s\n%s", bar, strings.Join(lines, "\n"), bar)
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

func main() {
	defaultCfg := config.DefaultConfigPath()
	configPath := flag.String("config", defaultCfg, fmt.Sprintf("path to config file (default %s)", defaultCfg))
	showVer := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVer {
		log.Printf("liapi %s", version)
		return
	}

	// Normalize: an empty -config falls back to the default path, and relative
	// paths are pinned to an absolute location so admin-API saves (atomic
	// rename next to the file) always target this same file.
	if strings.TrimSpace(*configPath) == "" {
		*configPath = config.DefaultConfigPath()
	}
	if abs, err := filepath.Abs(*configPath); err == nil {
		*configPath = abs
	}

	// Best effort: the data dir is only strictly needed for default paths.
	// Operators pointing -config elsewhere (Docker's /data, read-only HOME)
	// keep working — the file owners (logger/config save) create dirs as needed.
	if err := config.EnsureDataDir(); err != nil {
		log.Printf("[warn] cannot create data dir %s: %v (fine if -config and log paths are elsewhere)", config.DataDir(), err)
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
		Addr:              cfg.ListenAddr(),
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

	log.Printf("liapi %s listening on %s (version=%s)", version, cfg.ListenAddr(), version)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}
