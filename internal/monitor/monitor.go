// Package monitor は VM の使用量を定期的に取得して保存する。
package monitor

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/akatuki77/proxconductor/internal/proxmox"
	"github.com/akatuki77/proxconductor/internal/store"
)

// 使用量の保存先。
type MetricStore interface {
	SaveMetrics(ctx context.Context, ms []store.Metric) error
	DeleteMetricsBefore(ctx context.Context, before time.Time) (int64, error)
}

// 最後に取得した結果。
type Snapshot struct {
	VMs       []proxmox.VM
	UpdatedAt time.Time
	Err       string
}

type Collector struct {
	client    proxmox.Client
	store     MetricStore
	interval  time.Duration
	retention time.Duration
	logger    *slog.Logger
	onUpdate  func(Snapshot) // 取得のたびに呼ぶ。nil なら呼ばない
	now       func() time.Time

	mu     sync.RWMutex
	latest Snapshot

	lastCleanup time.Time
}

func New(client proxmox.Client, st MetricStore, interval, retention time.Duration, logger *slog.Logger, onUpdate func(Snapshot)) *Collector {
	return &Collector{
		client: client, store: st, interval: interval, retention: retention,
		logger: logger, onUpdate: onUpdate, now: time.Now,
	}
}

// Run は ctx が止まるまで interval ごとに取得する。
func (c *Collector) Run(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		c.CollectOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// CollectOnce は1回分を取得して保存する。
func (c *Collector) CollectOnce(ctx context.Context) {
	now := c.now()
	vms, err := c.client.ListVMs(ctx)
	if err != nil {
		c.logger.Warn("VM一覧の取得に失敗", "err", err)
		c.mu.Lock()
		c.latest.Err = err.Error()
		c.mu.Unlock()
		c.notify()
		return
	}

	c.mu.Lock()
	c.latest = Snapshot{VMs: vms, UpdatedAt: now}
	c.mu.Unlock()
	c.notify()

	ms := make([]store.Metric, 0, len(vms))
	for _, vm := range vms {
		if !vm.Running() {
			continue
		}
		ms = append(ms, store.Metric{
			VMID: vm.VMID, Time: now, CPU: vm.CPU, CPUs: vm.MaxCPU, MemUsed: vm.Mem, MemMax: vm.MaxMem,
		})
	}
	if err := c.store.SaveMetrics(ctx, ms); err != nil {
		c.logger.Warn("使用量の保存に失敗", "err", err)
	}
	c.logger.Debug("使用量を取得", "vms", len(vms), "saved", len(ms))

	if now.Sub(c.lastCleanup) >= time.Hour {
		c.lastCleanup = now
		n, err := c.store.DeleteMetricsBefore(ctx, now.Add(-c.retention))
		if err != nil {
			c.logger.Warn("古い使用量の削除に失敗", "err", err)
		} else if n > 0 {
			c.logger.Info("古い使用量を削除", "count", n)
		}
	}
}

// notify は最新の結果を onUpdate に渡す。
func (c *Collector) notify() {
	if c.onUpdate != nil {
		c.onUpdate(c.Latest())
	}
}

// Latest は最後に取得した結果のコピーを返す。
func (c *Collector) Latest() Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s := c.latest
	s.VMs = append([]proxmox.VM(nil), c.latest.VMs...)
	return s
}
