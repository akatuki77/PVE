// Package web は HTTP のルーティングとハンドラをまとめる。
package web

import (
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/akatuki77/proxconductor/internal/monitor"
	"github.com/akatuki77/proxconductor/internal/store"
)

// DB は API が使う保存先。
type DB interface {
	PingContext(ctx context.Context) error
	Metrics(ctx context.Context, vmid int, since time.Time) ([]store.Metric, error)
}

// VMSource は最新の VM 一覧を返す。
type VMSource interface {
	Latest() monitor.Snapshot
}

// Options は Server に渡すもの。
type Options struct {
	Logger *slog.Logger
	DB     DB
	VMs    VMSource
	Broker *Broker
	Static fs.FS // nil なら画面は配信しない
}

// Server はハンドラが使う依存を持つ。
type Server struct {
	logger *slog.Logger
	db     DB
	vms    VMSource
	broker *Broker
	static fs.FS
	now    func() time.Time
}

// NewServer は Server を作る。
func NewServer(o Options) *Server {
	return &Server{logger: o.Logger, db: o.DB, vms: o.VMs, broker: o.Broker, static: o.Static, now: time.Now}
}

// Handler はルーティング済みの http.Handler を返す。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/healthz", s.handleHealthz)
	mux.HandleFunc("GET /api/vms", s.handleVMs)
	mux.HandleFunc("GET /api/vms/{vmid}/metrics", s.handleMetrics)
	mux.HandleFunc("GET /api/stream", s.handleStream)
	if s.static != nil {
		mux.Handle("GET /", spaHandler(s.static))
	}
	return mux
}

type healthzResponse struct {
	Status string `json:"status"`
	DB     string `json:"db"`
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if err := s.db.PingContext(r.Context()); err != nil {
		s.logger.Error("DBの確認に失敗", "err", err)
		s.writeJSON(w, http.StatusServiceUnavailable, healthzResponse{Status: "error", DB: "error"})
		return
	}
	s.writeJSON(w, http.StatusOK, healthzResponse{Status: "ok", DB: "ok"})
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.logger.Warn("レスポンスの書き込みに失敗", "err", err)
	}
}

func (s *Server) writeError(w http.ResponseWriter, status int, msg string) {
	s.writeJSON(w, status, map[string]string{"error": msg})
}
