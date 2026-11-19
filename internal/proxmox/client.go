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
}

// Running は VM が起動中かを返す。
func (v VM) Running() bool { return v.Status == "running" }

// Client は Proxmox API の操作。
type Client interface {
	ListVMs(ctx context.Context) ([]VM, error)
}
