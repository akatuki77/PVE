// Package proxmox は Proxmox VE API と通信する。
package proxmox

import "context"

// VM は VM 一覧の1台分。
type VM struct {
	VMID   int      `json:"vmid"`
	Name   string   `json:"name"`
	Node   string   `json:"node"`
	Status string   `json:"status"`
	CPU    float64  `json:"cpu"`    // 使用率（0〜1）
	MaxCPU int      `json:"maxcpu"` // CPU数
	Mem    int64    `json:"mem"`    // 使用量（バイト）
	MaxMem int64    `json:"maxmem"` // 割り当て（バイト）
	Tags   []string `json:"tags"`

	Balloon  int64   `json:"balloon"`  // バルーンで今使える量（バイト）。0 なら不明
	CPULimit float64 `json:"cpulimit"` // CPU の上限（コア数）。0 なら上限なし
	SwapOut  int64   `json:"swap_out"` // VM の中でスワップに逃がした量の累計（バイト）
}

// Running は VM が起動中かを返す。
func (v VM) Running() bool { return v.Status == "running" }

// MemLimit は今使えるメモリの量（バイト）を返す。
func (v VM) MemLimit() int64 {
	if v.Balloon > 0 && v.Balloon < v.MaxMem {
		return v.Balloon
	}
	return v.MaxMem
}

// CPULimitCores は今使える CPU の量（コア数）を返す。
func (v VM) CPULimitCores() float64 {
	return effectiveCPULimit(v.CPULimit, v.MaxCPU)
}

func effectiveCPULimit(limit float64, cores int) float64 {
	if limit > 0 && limit < float64(cores) {
		return limit
	}
	return float64(cores)
}

// Client は Proxmox API の操作。
type Client interface {
	ListVMs(ctx context.Context) ([]VM, error)
	GetVMStatus(ctx context.Context, node string, vmid int) (VMStatus, error)
	GetVMConfig(ctx context.Context, node string, vmid int) (VMConfig, error)
	SetBalloon(ctx context.Context, node string, vmid int, mb int) error
	SetCPULimit(ctx context.Context, node string, vmid int, cores float64) error
}
