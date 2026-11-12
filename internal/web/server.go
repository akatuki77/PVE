// Package web は HTTP のルーティングとハンドラをまとめる。
package web

import (
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
)

// Pinger はデータベースが使えるかを確かめる。
type Pinger interface {
	PingContext(ctx context.Context) error
}

// Server はハンドラが使う依存を持つ。
type Server struct {
	logger *slog.Logger
	db     Pinger
	static fs.FS
}

// NewServer は Server を作る。static が nil なら画面は配信しない。
func NewServer(logger *slog.Logger, db Pinger, static fs.FS) *Server {
	return &Server{logger: logger, db: db, static: static}
}

// Handler はルーティング済みの http.Handler を返す。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/healthz", s.handleHealthz)
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
