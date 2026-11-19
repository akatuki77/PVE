// Command mock は開発用の偽 Proxmox VE API。デモ用のVM3台があるふりをする。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
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
	VMID     int
	Name     string
	Node     string
	Tags     string
	Status   string
	VCPUs    int
	MemoryMB int
	CPU      float64 // 平均の使用率（0〜1）
	MemUsed  int64   // 平均の使用量（バイト）
	Phase    float64 // 揺れのずれ
}

// usage は時刻に応じて揺れる使用量を返す。
func (v *vm) usage(now time.Time) (cpu float64, mem int64) {
	if v.Status != "running" {
		return 0, 0
	}
	t := float64(now.Unix())
	cpu = v.CPU + 0.15*math.Sin(t/60+v.Phase) + 0.03*math.Sin(t/7+v.Phase)
	cpu = math.Max(0.01, math.Min(1, cpu))
	memRatio := 1 + 0.08*math.Sin(t/180+v.Phase)
	mem = min(int64(float64(v.MemUsed)*memRatio), int64(v.MemoryMB)*mib)
	return cpu, mem
}

type cluster struct {
	mu  sync.Mutex
	vms []*vm
}

func newCluster() *cluster {
	return &cluster{vms: []*vm{
		{VMID: 101, Name: "db", Node: "pve1", Tags: "demo", Status: "running", VCPUs: 2, MemoryMB: 2048, CPU: 0.25, MemUsed: 1100 * mib, Phase: 0},
		{VMID: 102, Name: "app", Node: "pve2", Tags: "demo", Status: "running", VCPUs: 2, MemoryMB: 2048, CPU: 0.40, MemUsed: 900 * mib, Phase: 2},
		{VMID: 103, Name: "web", Node: "pve3", Tags: "demo", Status: "running", VCPUs: 1, MemoryMB: 1024, CPU: 0.15, MemUsed: 400 * mib, Phase: 4},
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

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "PVEAPIToken=") {
			http.Error(w, "authentication failure", http.StatusUnauthorized)
			return
		}
		api.ServeHTTP(w, r)
	})
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
			"cpu": cpu, "maxcpu": v.VCPUs, "mem": mem, "maxmem": int64(v.MemoryMB) * mib,
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
	writeData(w, map[string]any{
		"vmid": v.VMID, "name": v.Name, "status": v.Status, "cpus": v.VCPUs, "cpu": cpu,
		"mem": mem, "maxmem": int64(v.MemoryMB) * mib, "tags": v.Tags,
	})
}
