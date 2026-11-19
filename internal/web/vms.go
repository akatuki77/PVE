package web

import (
	"net/http"
	"strconv"
	"time"

	"github.com/akatuki77/proxconductor/internal/proxmox"
	"github.com/akatuki77/proxconductor/internal/store"
)

// VMsResponse は GET /api/vms の応答。
type VMsResponse struct {
	VMs       []proxmox.VM `json:"vms"`
	UpdatedAt *time.Time   `json:"updated_at"` // まだ取得していなければ null
	Error     string       `json:"error"`      // 最後の取得のエラー
}

func (s *Server) handleVMs(w http.ResponseWriter, _ *http.Request) {
	snap := s.vms.Latest()
	res := VMsResponse{VMs: snap.VMs, Error: snap.Err}
	if res.VMs == nil {
		res.VMs = []proxmox.VM{}
	}
	if !snap.UpdatedAt.IsZero() {
		res.UpdatedAt = &snap.UpdatedAt
	}
	s.writeJSON(w, http.StatusOK, res)
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
