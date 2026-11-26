// Command mock は開発用の偽 Proxmox VE API。デモ用のVM3台があるふりをする。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const mib = 1024 * 1024

type vm struct {
	VMID      int
	Name      string
	Node      string
	Tags      string
	Status    string
	Cores     int
	CPULimit  float64 // CPU の上限（コア数）。0 なら上限なし
	MemoryMB  int     // メモリの設定値
	BalloonMB int     // バルーンで今使える量
	CPU       float64 // 欲しい CPU（全コアに対する割合 0〜1）
	MemUsed   int64   // 欲しいメモリ（バイト）
	Phase     float64 // 揺れのずれ
	SwapOut   int64   // スワップに逃がした量の累計（バイト）
}

// usage は時刻に応じて揺れる使用量を返す。上限やバルーンを超えては使えない。
func (v *vm) usage(now time.Time) (cpu float64, mem int64) {
	if v.Status != "running" {
		return 0, 0
	}
	limit := 1.0
	if v.CPULimit > 0 && v.CPULimit < float64(v.Cores) {
		limit = v.CPULimit / float64(v.Cores)
	}
	t := float64(now.Unix())
	cpu = v.CPU + 0.05*math.Sin(t/60+v.Phase) + 0.02*math.Sin(t/7+v.Phase)
	cpu = math.Max(0.01, math.Min(limit, cpu))
	memRatio := 1 + 0.03*math.Sin(t/180+v.Phase)
	// 本物と同じく、使える量の 80% ほどで止まり、あふれた分はスワップに逃がす
	mem = min(int64(float64(v.MemUsed)*memRatio), v.memCap())
	return cpu, mem
}

func (v *vm) memCap() int64 { return int64(v.BalloonMB) * mib * 8 / 10 }

type cluster struct {
	mu  sync.Mutex
	vms []*vm
}

func newCluster() *cluster {
	return &cluster{vms: []*vm{
		{VMID: 101, Name: "db", Node: "pve1", Tags: "demo", Status: "running", Cores: 4, CPULimit: 2, MemoryMB: 4096, BalloonMB: 2048, CPU: 0.25, MemUsed: 1100 * mib, Phase: 0},
		{VMID: 102, Name: "app", Node: "pve2", Tags: "demo", Status: "running", Cores: 4, CPULimit: 2, MemoryMB: 4096, BalloonMB: 2048, CPU: 0.20, MemUsed: 900 * mib, Phase: 2},
		{VMID: 103, Name: "web", Node: "pve3", Tags: "demo", Status: "running", Cores: 2, CPULimit: 1, MemoryMB: 2048, BalloonMB: 1024, CPU: 0.15, MemUsed: 400 * mib, Phase: 4},
	}}
}

func (c *cluster) find(node string, vmid int) *vm {
	for _, v := range c.vms {
		if v.VMID == vmid && v.Node == node {
			return v
		}
	}
	return nil
}

func main() {
	addr := flag.String("addr", ":8006", "待ち受けるアドレス")
	flag.Parse()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := &http.Server{Addr: *addr, Handler: newCluster().handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
	}()

	logger.Info("偽 Proxmox API を起動しました", "addr", *addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("起動に失敗", "err", err)
		os.Exit(1)
	}
}

func (c *cluster) handler() http.Handler {
	api := http.NewServeMux()
	api.HandleFunc("GET /api2/json/cluster/resources", c.handleResources)
	api.HandleFunc("GET /api2/json/nodes/{node}/qemu/{vmid}/status/current", c.handleStatus)
	api.HandleFunc("GET /api2/json/nodes/{node}/qemu/{vmid}/config", c.handleConfig)
	api.HandleFunc("PUT /api2/json/nodes/{node}/qemu/{vmid}/config", c.handleSetConfig)
	api.HandleFunc("POST /api2/json/nodes/{node}/qemu/{vmid}/monitor", c.handleMonitor)

	mux := http.NewServeMux()
	mux.Handle("/api2/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "PVEAPIToken=") {
			http.Error(w, "authentication failure", http.StatusUnauthorized)
			return
		}
		api.ServeHTTP(w, r)
	}))
	// 開発用。本物の Proxmox にはない
	mux.HandleFunc("POST /mock/load", c.handleLoad)
	return mux
}

func writeData(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"data": data})
}

func (c *cluster) handleResources(w http.ResponseWriter, r *http.Request) {
	if t := r.URL.Query().Get("type"); t != "" && t != "vm" {
		writeData(w, []any{})
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	list := make([]map[string]any, 0, len(c.vms))
	for _, v := range c.vms {
		cpu, mem := v.usage(now)
		list = append(list, map[string]any{
			"id": "qemu/" + strconv.Itoa(v.VMID), "type": "qemu", "vmid": v.VMID, "name": v.Name,
			"node": v.Node, "status": v.Status, "tags": v.Tags, "template": 0,
			"cpu": cpu, "maxcpu": v.Cores, "mem": mem, "maxmem": int64(v.MemoryMB) * mib,
		})
	}
	writeData(w, list)
}

func (c *cluster) handleStatus(w http.ResponseWriter, r *http.Request) {
	vmid, _ := strconv.Atoi(r.PathValue("vmid"))
	c.mu.Lock()
	defer c.mu.Unlock()
	v := c.find(r.PathValue("node"), vmid)
	if v == nil {
		http.Error(w, "Configuration file does not exist", http.StatusInternalServerError)
		return
	}
	cpu, mem := v.usage(time.Now())
	if over := v.MemUsed - v.memCap(); v.Status == "running" && over > 0 {
		v.SwapOut += over / 10
	}
	writeData(w, map[string]any{
		"vmid": v.VMID, "name": v.Name, "status": v.Status, "cpus": v.Cores, "cpu": cpu,
		"mem": mem, "maxmem": int64(v.MemoryMB) * mib, "balloon": int64(v.BalloonMB) * mib, "tags": v.Tags,
		"ballooninfo": map[string]any{"mem_swapped_out": v.SwapOut},
	})
}

func (c *cluster) handleConfig(w http.ResponseWriter, r *http.Request) {
	vmid, _ := strconv.Atoi(r.PathValue("vmid"))
	c.mu.Lock()
	defer c.mu.Unlock()
	v := c.find(r.PathValue("node"), vmid)
	if v == nil {
		http.Error(w, "Configuration file does not exist", http.StatusInternalServerError)
		return
	}
	// 本物に合わせて memory は文字列で返す
	data := map[string]any{
		"name": v.Name, "cores": v.Cores, "sockets": 1, "memory": strconv.Itoa(v.MemoryMB), "tags": v.Tags,
	}
	if v.CPULimit > 0 {
		data["cpulimit"] = strconv.FormatFloat(v.CPULimit, 'f', -1, 64)
	}
	writeData(w, data)
}

// handleSetConfig は PUT .../config。今は cpulimit だけ変えられる。
func (c *cluster) handleSetConfig(w http.ResponseWriter, r *http.Request) {
	vmid, _ := strconv.Atoi(r.PathValue("vmid"))
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	v := c.find(r.PathValue("node"), vmid)
	if v == nil {
		http.Error(w, "Configuration file does not exist", http.StatusInternalServerError)
		return
	}
	if s := r.PostForm.Get("cpulimit"); s != "" {
		n, err := strconv.ParseFloat(s, 64)
		if err != nil || n < 0 || n > 128 {
			http.Error(w, "cpulimit: invalid value", http.StatusBadRequest)
			return
		}
		v.CPULimit = n
	}
	writeData(w, nil)
}

// handleMonitor は POST .../monitor。今は "balloon N" だけ受け付ける。
func (c *cluster) handleMonitor(w http.ResponseWriter, r *http.Request) {
	vmid, _ := strconv.Atoi(r.PathValue("vmid"))
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	v := c.find(r.PathValue("node"), vmid)
	if v == nil {
		http.Error(w, "Configuration file does not exist", http.StatusInternalServerError)
		return
	}
	var mb int
	if _, err := fmt.Sscanf(r.PostForm.Get("command"), "balloon %d", &mb); err != nil {
		writeData(w, "unknown command")
		return
	}
	v.BalloonMB = max(1, min(mb, v.MemoryMB))
	writeData(w, "")
}

type loadRequest struct {
	VMID int      `json:"vmid"`
	CPU  *float64 `json:"cpu"`    // 平均の CPU 使用率（0〜1）
	Mem  *int     `json:"mem_mb"` // 平均のメモリ使用量（MB）
}

// handleLoad は POST /mock/load。VM の負荷を変える（stress-ng の代わり）。
func (c *cluster) handleLoad(w http.ResponseWriter, r *http.Request) {
	var req loadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, v := range c.vms {
		if v.VMID != req.VMID {
			continue
		}
		if req.CPU != nil {
			v.CPU = *req.CPU
		}
		if req.Mem != nil {
			v.MemUsed = int64(*req.Mem) * mib
		}
		writeData(w, map[string]any{"vmid": v.VMID, "cpu": v.CPU, "mem_mb": v.MemUsed / mib})
		return
	}
	http.Error(w, "no such vm", http.StatusNotFound)
}
