package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/akatuki77/proxconductor/internal/monitor"
	"github.com/akatuki77/proxconductor/internal/proxmox"
	"github.com/akatuki77/proxconductor/internal/store"
	"github.com/akatuki77/proxconductor/internal/web"
	webui "github.com/akatuki77/proxconductor/web"
)

// config は起動時の設定。
type config struct {
	addr      string
	dbPath    string
	interval  time.Duration
	retention time.Duration
	debug     bool

	pveURL         string
	pveTokenID     string
	pveTokenSecret string
	pveInsecure    bool
}

func main() {
	var cfg config
	flag.StringVar(&cfg.addr, "addr", ":8080", "待ち受けるアドレス")
	flag.StringVar(&cfg.dbPath, "db", "data/proxconductor.db", "SQLite のファイル")
	flag.DurationVar(&cfg.interval, "interval", 15*time.Second, "使用量を取得する間隔")
	flag.DurationVar(&cfg.retention, "retention", 24*time.Hour, "使用量を残す期間")
	flag.BoolVar(&cfg.debug, "debug", false, "詳しいログを出す")
	flag.Parse()

	level := slog.LevelInfo
	if cfg.debug {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	if err := loadEnv(&cfg); err != nil {
		logger.Error("設定が不足しています", "err", err)
		os.Exit(1)
	}
	if err := run(cfg, logger); err != nil {
		logger.Error("異常終了", "err", err)
		os.Exit(1)
	}
}

func loadEnv(cfg *config) error {
	cfg.pveURL = os.Getenv("PVE_API_URL")
	cfg.pveTokenID = os.Getenv("PVE_TOKEN_ID")
	cfg.pveTokenSecret = os.Getenv("PVE_TOKEN_SECRET")
	if cfg.pveURL == "" || cfg.pveTokenID == "" || cfg.pveTokenSecret == "" {
		return errors.New("PVE_API_URL・PVE_TOKEN_ID・PVE_TOKEN_SECRET を設定してください")
	}
	if v := os.Getenv("PVE_TLS_INSECURE"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("PVE_TLS_INSECURE は true か false にしてください: %w", err)
		}
		cfg.pveInsecure = b
	}
	return nil
}

func run(cfg config, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	static, err := webui.Dist()
	if err != nil {
		return fmt.Errorf("画面ファイルの読み込みに失敗: %w", err)
	}

	client := proxmox.NewHTTPClient(cfg.pveURL, cfg.pveTokenID, cfg.pveTokenSecret, cfg.pveInsecure)
	collector := monitor.New(client, st, cfg.interval, cfg.retention, logger)

	srv := &http.Server{
		Addr: cfg.addr,
		Handler: web.NewServer(web.Options{
			Logger: logger, DB: st, VMs: collector, Static: static,
		}).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		collector.Run(ctx)
	}()

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("起動しました", "addr", cfg.addr, "db", cfg.dbPath,
			"pve", cfg.pveURL, "interval", cfg.interval)
		serveErr <- srv.ListenAndServe()
	}()

	var runErr error
	select {
	case err := <-serveErr:
		runErr = fmt.Errorf("HTTPサーバーが停止: %w", err)
		stop()
	case <-ctx.Done():
		logger.Info("終了します")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Warn("HTTPサーバーの停止に失敗", "err", err)
	}
	wg.Wait()
	return runErr
}
