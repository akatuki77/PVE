package web

import (
	"context"
	"net/http"
	"strconv"

	"github.com/akatuki77/proxconductor/internal/store"
)

// Record は記録を保存し、ログに出して画面に送る。
func (s *Server) Record(ctx context.Context, e store.Event) {
	s.logger.Info("記録", "vmid", e.VMID, "kind", e.Kind, "dry_run", e.DryRun, "message", e.Message)
	saved, err := s.db.AddEvent(ctx, e)
	if err != nil {
		s.logger.Warn("記録の保存に失敗", "err", err)
		saved = e
	}
	// 記録が増えるときに、ついでに古いものを消す
	if s.eventRetention > 0 {
		if _, err := s.db.DeleteEventsBefore(ctx, s.now().Add(-s.eventRetention)); err != nil {
			s.logger.Warn("古い記録の削除に失敗", "err", err)
		}
	}
	if err := s.broker.Publish("event", saved); err != nil {
		s.logger.Warn("画面への通知に失敗", "err", err)
	}
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			s.writeError(w, http.StatusBadRequest, "limit は 1〜500 にしてください")
			return
		}
		limit = n
	}
	events, err := s.db.Events(r.Context(), limit)
	if err != nil {
		s.logger.Error("記録の取得に失敗", "err", err)
		s.writeError(w, http.StatusInternalServerError, "記録の取得に失敗しました")
		return
	}
	s.writeJSON(w, http.StatusOK, events)
}
