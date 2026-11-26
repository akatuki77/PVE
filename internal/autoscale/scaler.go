package autoscale

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/akatuki77/proxconductor/internal/config"
	"github.com/akatuki77/proxconductor/internal/monitor"
	"github.com/akatuki77/proxconductor/internal/proxmox"
	"github.com/akatuki77/proxconductor/internal/store"
)

const mib = 1024 * 1024

// VMSource は最新の VM 一覧を返す。
type VMSource interface {
	Latest() monitor.Snapshot
}

// DB はスケールに使う保存先。
type DB interface {
	Targets(ctx context.Context) ([]store.Target, error)
	Metrics(ctx context.Context, vmid int, since time.Time) ([]store.Metric, error)
}

// Client は VM の読み取りと変更に使う Proxmox API。
type Client interface {
	GetVMConfig(ctx context.Context, node string, vmid int) (proxmox.VMConfig, error)
	GetVMStatus(ctx context.Context, node string, vmid int) (proxmox.VMStatus, error)
	SetBalloon(ctx context.Context, node string, vmid int, mb int) error
	SetCPULimit(ctx context.Context, node string, vmid int, cores float64) error
}

// Recorder は判定と実行の結果を記録する。
type Recorder interface {
	Record(ctx context.Context, e store.Event)
}

// Scaler は対象の VM を定期的に判定し、dryRun でなければ実際に変える。
type Scaler struct {
	cfg      config.Autoscale
	dryRun   bool
	vms      VMSource
	db       DB
	client   Client
	recorder Recorder
	logger   *slog.Logger
	now      func() time.Time

	verifyTimeout  time.Duration // バルーンが目標に届くまで待つ時間
	verifyInterval time.Duration // その間に確かめる間隔

	// Run の goroutine の中だけで使う
	lastAction map[int]time.Time // VM ごとの最後に変更を決めた時刻
	warned     map[int]bool      // 名前が違う警告を出した VM
	lastSwap   map[int]int64     // VM ごとの前回のスワップの累計
	swapGrewAt map[int]time.Time // VM ごとの最後にスワップが増えた時刻
}

// New は Scaler を作る。
func New(cfg config.Autoscale, dryRun bool, vms VMSource, db DB, client Client, recorder Recorder, logger *slog.Logger) *Scaler {
	return &Scaler{
		cfg: cfg, dryRun: dryRun, vms: vms, db: db, client: client, recorder: recorder, logger: logger,
		now: time.Now, verifyTimeout: 15 * time.Second, verifyInterval: time.Second,
		lastAction: map[int]time.Time{}, warned: map[int]bool{},
		lastSwap: map[int]int64{}, swapGrewAt: map[int]time.Time{},
	}
}

// Run は ctx が止まるまで interval ごとに判定する。
func (s *Scaler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.RunOnce(ctx)
		}
	}
}

// RunOnce は対象の VM をすべて1回判定する。
func (s *Scaler) RunOnce(ctx context.Context) {
	targets, err := s.db.Targets(ctx)
	if err != nil {
		s.logger.Warn("対象VMの取得に失敗", "err", err)
		return
	}
	vms := s.vms.Latest().VMs
	byID := make(map[int]proxmox.VM, len(vms))
	for _, vm := range vms {
		byID[vm.VMID] = vm
	}
	now := s.now()

	for _, t := range targets {
		vm, ok := byID[t.VMID]
		if !ok || !vm.Running() {
			continue
		}
		if vm.Name != t.Name {
			// VMID の使い回しで別の VM を変えないように止める。警告は1回だけ
			if !s.warned[t.VMID] {
				s.warned[t.VMID] = true
				s.record(ctx, now, t.VMID, true, fmt.Sprintf("VM名が %q から %q に変わったため、オートスケールを止めました。画面で選び直してください", t.Name, vm.Name))
			}
			continue
		}
		delete(s.warned, t.VMID)
		// スワップはクールダウン中も見ておく（累計なので、前回より増えたかで判断する）
		if prev, ok := s.lastSwap[t.VMID]; ok && vm.SwapOut > prev {
			s.swapGrewAt[t.VMID] = now
		}
		s.lastSwap[t.VMID] = vm.SwapOut
		if last, ok := s.lastAction[t.VMID]; ok && now.Sub(last) < s.cfg.Cooldown {
			continue
		}
		if err := s.check(ctx, vm, now); err != nil {
			s.logger.Warn("オートスケールの判定に失敗", "vmid", vm.VMID, "err", err)
		}
	}
}

// check は1台を判定し、変更があれば実行（ドライランなら記録だけ）する。
func (s *Scaler) check(ctx context.Context, vm proxmox.VM, now time.Time) error {
	samples, err := s.db.Metrics(ctx, vm.VMID, now.Add(-s.cfg.Window))
	if err != nil {
		return err
	}
	vmCfg, err := s.client.GetVMConfig(ctx, vm.Node, vm.VMID)
	if err != nil {
		return err
	}
	vmSt, err := s.client.GetVMStatus(ctx, vm.Node, vm.VMID)
	if err != nil {
		return err
	}
	state := State{
		CPULimit: vmCfg.CurrentCPULimit(), Cores: vmCfg.Cores,
		MemoryMB: int(vmSt.MemLimit() / mib), MaxMemoryMB: vmCfg.MemoryMB, UsedMB: int(vmSt.Mem / mib),
	}
	// window の間にスワップが増えていたら、メモリが足りないとみなす
	if grewAt, ok := s.swapGrewAt[vm.VMID]; ok && now.Sub(grewAt) <= s.cfg.Window {
		state.SwapGrowing = true
	}
	d := Decide(s.cfg.PolicyFor(vm.VMID), state, samples)
	if !d.Changed() {
		return nil
	}
	// 変更を決めたらクールダウンを始め、同じ判定を続けて出さない
	s.lastAction[vm.VMID] = now
	if s.dryRun {
		s.record(ctx, now, vm.VMID, true, d.Reason)
		return nil
	}
	s.record(ctx, now, vm.VMID, false, d.Reason+"："+s.apply(ctx, vm, d))
	return nil
}

// apply は判定のとおりに変え、反映されたかを確かめて、結果の文を返す。
func (s *Scaler) apply(ctx context.Context, vm proxmox.VM, d Decision) string {
	var results []string
	if d.NewCPULimit != 0 {
		results = append(results, s.applyCPU(ctx, vm, d.NewCPULimit))
	}
	if d.NewMemoryMB != 0 {
		results = append(results, s.applyMemory(ctx, vm, d.NewMemoryMB))
	}
	return strings.Join(results, "、")
}

func (s *Scaler) applyCPU(ctx context.Context, vm proxmox.VM, cores float64) string {
	if err := s.client.SetCPULimit(ctx, vm.Node, vm.VMID, cores); err != nil {
		return "CPU の変更に失敗（" + err.Error() + "）"
	}
	cfg, err := s.client.GetVMConfig(ctx, vm.Node, vm.VMID)
	if err != nil {
		return "CPU を変更（確認に失敗：" + err.Error() + "）"
	}
	if math.Abs(cfg.CurrentCPULimit()-cores) > 0.01 {
		return fmt.Sprintf("CPU を変更したが反映を確認できず（今 %g コア）", cfg.CurrentCPULimit())
	}
	return "CPU を変更しました"
}

// applyMemory はバルーンを変える。中の OS が少しずつ合わせるので、届くまで待って確かめる。
func (s *Scaler) applyMemory(ctx context.Context, vm proxmox.VM, mb int) string {
	if err := s.client.SetBalloon(ctx, vm.Node, vm.VMID, mb); err != nil {
		return "メモリの変更に失敗（" + err.Error() + "）"
	}
	deadline := time.Now().Add(s.verifyTimeout)
	var lastMB int
	for {
		st, err := s.client.GetVMStatus(ctx, vm.Node, vm.VMID)
		if err != nil {
			return "メモリを変更（確認に失敗：" + err.Error() + "）"
		}
		lastMB = int(st.MemLimit() / mib)
		if abs(lastMB-mb) <= mb/20 { // 5% 以内なら届いたとみなす
			return "メモリを変更しました"
		}
		if time.Now().After(deadline) {
			return fmt.Sprintf("メモリを変更したが目標に届かず（今 %d MB）", lastMB)
		}
		select {
		case <-ctx.Done():
			return "メモリを変更（確認の途中で終了）"
		case <-time.After(s.verifyInterval):
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func (s *Scaler) record(ctx context.Context, now time.Time, vmid int, dryRun bool, msg string) {
	s.recorder.Record(ctx, store.Event{Time: now, VMID: vmid, Kind: "autoscale", Message: msg, DryRun: dryRun})
}
