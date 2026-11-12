// Command proxconductor は Proxmox VE 向けの運用支援ツールを起動する。
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
	"syscall"
	"time"

	"github.com/akatuki77/proxconductor/internal/store"
	"github.com/akatuki77/proxconductor/internal/web"
	webui "github.com/akatuki77/proxconductor/web"
)

func main() {
	addr := flag.String("addr", ":8080", "待ち受けるアドレス")
	dbPath := flag.String("db", "data/proxconductor.db", "SQLite のファイル")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(*addr, *dbPath, logger); err != nil {
		logger.Error("異常終了", "err", err)
		os.Exit(1)
	}
}

func run(addr, dbPath string, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	static, err := webui.Dist()
	if err != nil {
		return fmt.Errorf("画面ファイルの読み込みに失敗: %w", err)
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           web.NewServer(logger, st, static).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("起動しました", "addr", addr, "db", dbPath)
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		return fmt.Errorf("HTTPサーバーが停止: %w", err)
	case <-ctx.Done():
		logger.Info("終了します")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("HTTPサーバーの停止に失敗: %w", err)
	}
	return nil
}
