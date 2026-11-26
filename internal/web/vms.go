package web

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/akatuki77/proxconductor/internal/monitor"
	"github.com/akatuki77/proxconductor/internal/proxmox"
	"github.com/akatuki77/proxconductor/internal/store"
)

// VMView は画面に返す VM 1台分。
type VMView struct {
	proxmox.VM
	Autoscale         bool `json:"autoscale"`          // オートスケールの対象か
	AutoscaleMismatch bool `json:"autoscale_mismatch"` // 選んだときと VM 名が違う
}

// VMsResponse は GET /api/vms の応答。
type VMsResponse struct {
	VMs       []VMView   `json:"vms"`
	UpdatedAt *time.Time `json:"updated_at"` // まだ取得していなければ null
	Error     string     `json:"error"`      // 最後の取得のエラー
}

// vmsResponse は取得結果と対象の VM から応答を作る。
func (s *Server) vmsResponse(ctx context.Context, snap monitor.Snapshot) (VMsResponse, error) {
	targets, err := s.db.Targets(ctx)
	if err != nil {
		return VMsResponse{}, err
	}
	names := make(map[int]string, len(targets))
	for _, t := range targets {
		names[t.VMID] = t.Name
	}

	res := VMsResponse{VMs: make([]VMView, 0, len(snap.VMs)), Error: snap.Err}
	for _, vm := range snap.VMs {
		name, ok := names[vm.VMID]
		res.VMs = append(res.VMs, VMView{VM: vm, Autoscale: ok, AutoscaleMismatch: ok && name != vm.Name})
	}
	if !snap.UpdatedAt.IsZero() {
		res.UpdatedAt = &snap.UpdatedAt
	}
	return res, nil
}

// PublishVMs は最新の VM 一覧を画面に送る。
func (s *Server) PublishVMs(ctx context.Context, snap monitor.Snapshot) {
	res, err := s.vmsResponse(ctx, snap)
	if err != nil {
		s.logger.Warn("VM一覧の作成に失敗", "err", err)
		return
	}
	if err := s.broker.Publish("vms", res); err != nil {
		s.logger.Warn("画面への通知に失敗", "err", err)
	}
}

func (s *Server) handleVMs(w http.ResponseWriter, r *http.Request) {
	res, err := s.vmsResponse(r.Context(), s.vms.Latest())
	if err != nil {
		s.logger.Error("VM一覧の作成に失敗", "err", err)
		s.writeError(w, http.StatusInternalServerError, "VM一覧の作成に失敗しました")
		return
	}
	s.writeJSON(w, http.StatusOK, res)
}

// handleAddTarget は PUT /api/vms/{vmid}/autoscale。VM をオートスケールの対象にする。
func (s *Server) handleAddTarget(w http.ResponseWriter, r *http.Request) {
	vmid, err := strconv.Atoi(r.PathValue("vmid"))
	if err != nil || vmid <= 0 {
		s.writeError(w, http.StatusBadRequest, "VMIDが正しくありません")
		return
	}
	snap := s.vms.Latest()
	var found *proxmox.VM
	for i := range snap.VMs {
		if snap.VMs[i].VMID == vmid {
			found = &snap.VMs[i]
			break
		}
	}
	if found == nil {
		s.writeError(w, http.StatusNotFound, "そのVMは見つかりません")
		return
	}
	if err := s.db.AddTarget(r.Context(), vmid, found.Name, s.now()); err != nil {
		s.logger.Error("対象VMの追加に失敗", "err", err)
		s.writeError(w, http.StatusInternalServerError, "対象VMの追加に失敗しました")
		return
	}
	s.logger.Info("オートスケールの対象にしました", "vmid", vmid, "name", found.Name)
	s.PublishVMs(r.Context(), snap)
	w.WriteHeader(http.StatusNoContent)
}

// handleRemoveTarget は DELETE /api/vms/{vmid}/autoscale。VM を対象から外す。
func (s *Server) handleRemoveTarget(w http.ResponseWriter, r *http.Request) {
	vmid, err := strconv.Atoi(r.PathValue("vmid"))
	if err != nil || vmid <= 0 {
		s.writeError(w, http.StatusBadRequest, "VMIDが正しくありません")
		return
	}
	if err := s.db.RemoveTarget(r.Context(), vmid); err != nil {
		s.logger.Error("対象VMの削除に失敗", "err", err)
		s.writeError(w, http.StatusInternalServerError, "対象VMの削除に失敗しました")
		return
	}
	s.logger.Info("オートスケールの対象から外しました", "vmid", vmid)
	s.PublishVMs(r.Context(), s.vms.Latest())
	w.WriteHeader(http.StatusNoContent)
}

// MetricPoint はグラフの1点。CPU・Mem は使用率（%）。
type MetricPoint struct {
	T       int64   `json:"t"` // UNIX時刻（ミリ秒）
	CPU     float64 `json:"cpu"`
	Mem     float64 `json:"mem"`
	CPUs    int     `json:"cpus"`
	MemUsed int64   `json:"mem_used"`
	MemMax  int64   `json:"mem_max"`
}

// MetricsResponse は GET /api/vms/{vmid}/metrics の応答。
type MetricsResponse struct {
	VMID   int           `json:"vmid"`
	Range  string        `json:"range"`
	Points []MetricPoint `json:"points"`
}

const maxPoints = 300

var ranges = map[string]time.Duration{
	"1h":  time.Hour,
	"6h":  6 * time.Hour,
	"24h": 24 * time.Hour,
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	vmid, err := strconv.Atoi(r.PathValue("vmid"))
	if err != nil || vmid <= 0 {
		s.writeError(w, http.StatusBadRequest, "VMIDが正しくありません")
		return
	}
	rangeKey := r.URL.Query().Get("range")
	if rangeKey == "" {
		rangeKey = "1h"
	}
	d, ok := ranges[rangeKey]
	if !ok {
		s.writeError(w, http.StatusBadRequest, "range は 1h・6h・24h のどれかにしてください")
		return
	}

	ms, err := s.db.Metrics(r.Context(), vmid, s.now().Add(-d))
	if err != nil {
		s.logger.Error("使用量の取得に失敗", "err", err)
		s.writeError(w, http.StatusInternalServerError, "使用量の取得に失敗しました")
		return
	}
	s.writeJSON(w, http.StatusOK, MetricsResponse{VMID: vmid, Range: rangeKey, Points: downsample(ms, maxPoints)})
}

// downsample は使用量を最大 limit 点に平均でまとめる。
func downsample(ms []store.Metric, limit int) []MetricPoint {
	points := []MetricPoint{}
	if len(ms) == 0 {
		return points
	}
	size := (len(ms) + limit - 1) / limit
	for i := 0; i < len(ms); i += size {
		chunk := ms[i:min(i+size, len(ms))]
		var cpu, mem float64
		var memUsed int64
		for _, m := range chunk {
			cpu += m.CPU * 100
			memUsed += m.MemUsed
			if m.MemMax > 0 {
				mem += float64(m.MemUsed) / float64(m.MemMax) * 100
			}
		}
		n := len(chunk)
		last := chunk[n-1]
		points = append(points, MetricPoint{
			T:       last.Time.UnixMilli(),
			CPU:     cpu / float64(n),
			Mem:     mem / float64(n),
			CPUs:    last.CPUs,
			MemUsed: memUsed / int64(n),
			MemMax:  last.MemMax,
		})
	}
	return points
}
