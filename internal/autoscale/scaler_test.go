package autoscale

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/akatuki77/proxconductor/internal/config"
	"github.com/akatuki77/proxconductor/internal/monitor"
	"github.com/akatuki77/proxconductor/internal/proxmox"
	"github.com/akatuki77/proxconductor/internal/store"
)

type fakeVMs []proxmox.VM

func (f fakeVMs) Latest() monitor.Snapshot { return monitor.Snapshot{VMs: f} }

type fakeDB struct {
	targets []store.Target
	samples []store.Metric
}

func (f fakeDB) Targets(context.Context) ([]store.Target, error) { return f.targets, nil }
func (f fakeDB) Metrics(context.Context, int, time.Time) ([]store.Metric, error) {
	return f.samples, nil
}

// fakeClient は偽の Proxmox。変更を受けると値が変わる（stuck なら変わらない）。
type fakeClient struct {
	cfg     proxmox.VMConfig
	st      proxmox.VMStatus
	stuck   bool  // true ならバルーンが目標に届かない
	failErr error // 変更の API が返すエラー
	calls   []string
}

func (f *fakeClient) GetVMConfig(context.Context, string, int) (proxmox.VMConfig, error) {
	return f.cfg, nil
}

func (f *fakeClient) GetVMStatus(context.Context, string, int) (proxmox.VMStatus, error) {
	return f.st, nil
}

func (f *fakeClient) SetBalloon(_ context.Context, _ string, _ int, mb int) error {
	f.calls = append(f.calls, "balloon")
	if f.failErr != nil {
		return f.failErr
	}
	if !f.stuck {
		f.st.Balloon = int64(mb) * mib
	}
	return nil
}

func (f *fakeClient) SetCPULimit(_ context.Context, _ string, _ int, cores float64) error {
	f.calls = append(f.calls, "cpulimit")
	if f.failErr != nil {
		return f.failErr
	}
	f.cfg.CPULimit = cores
	return nil
}

type fakeRecorder []store.Event

func (f *fakeRecorder) Record(_ context.Context, e store.Event) { *f = append(*f, e) }

func newClient() *fakeClient {
	return &fakeClient{
		cfg: proxmox.VMConfig{Cores: 4, MemoryMB: 4096, CPULimit: 2},
		st:  proxmox.VMStatus{Status: "running", Mem: 500 * mib, MaxMem: 4096 * mib, Balloon: 2048 * mib},
	}
}

func newScaler(client Client, db DB, vm proxmox.VM, dryRun bool, rec Recorder) *Scaler {
	s := New(config.Default().Autoscale, dryRun, fakeVMs{vm}, db, client, rec, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.verifyTimeout = 10 * time.Millisecond
	s.verifyInterval = time.Millisecond
	return s
}

func TestRunOnce(t *testing.T) {
	running := proxmox.VM{VMID: 101, Name: "db", Node: "pve1", Status: "running"}
	stopped := proxmox.VM{VMID: 101, Name: "db", Node: "pve1", Status: "stopped"}
	target := []store.Target{{VMID: 101, Name: "db"}}
	cpuHigh := samples(3, 95, 60)

	tests := []struct {
		name       string
		vm         proxmox.VM
		targets    []store.Target
		samples    []store.Metric
		lastAction time.Duration // 何分前に変更を決めたか（0 なら履歴なし）
		wantEvents int
		wantMsg    string
	}{
		{"条件を満たせば記録する", running, target, cpuHigh, 0, 1, "2 → 2.5 コア"},
		{"対象に選んでいなければ何もしない", running, nil, cpuHigh, 0, 0, ""},
		{"止まっているVMは何もしない", stopped, target, cpuHigh, 0, 0, ""},
		{"使用量が足りなければ何もしない", running, target, samples(2, 95, 60), 0, 0, ""},
		{"クールダウン中は何もしない", running, target, cpuHigh, 1 * time.Minute, 0, ""},
		{"クールダウン後なら記録する", running, target, cpuHigh, 10 * time.Minute, 1, "2 → 2.5 コア"},
		{"VM名が違えば止めて知らせる", running, []store.Target{{VMID: 101, Name: "old"}}, cpuHigh, 0, 1, "VM名が"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &fakeRecorder{}
			client := newClient()
			s := newScaler(client, fakeDB{targets: tt.targets, samples: tt.samples}, tt.vm, true, rec)
			now := time.Unix(1_700_000_000, 0)
			s.now = func() time.Time { return now }
			if tt.lastAction > 0 {
				s.lastAction[101] = now.Add(-tt.lastAction)
			}

			s.RunOnce(context.Background())

			if len(*rec) != tt.wantEvents {
				t.Fatalf("記録の数 = %d, want %d: %+v", len(*rec), tt.wantEvents, *rec)
			}
			if tt.wantEvents > 0 && !strings.Contains((*rec)[0].Message, tt.wantMsg) {
				t.Errorf("記録 = %+v", (*rec)[0])
			}
			if len(client.calls) != 0 {
				t.Errorf("ドライランでは変更しないはず: %v", client.calls)
			}
		})
	}
}

func TestApply(t *testing.T) {
	vm := proxmox.VM{VMID: 101, Name: "db", Node: "pve1", Status: "running"}
	target := []store.Target{{VMID: 101, Name: "db"}}

	tests := []struct {
		name      string
		samples   []store.Metric
		setup     func(c *fakeClient)
		wantCalls []string
		wantMsg   string
		wantCPU   float64 // 変更後の CPU の上限（0 なら確かめない）
		wantMemMB int64   // 変更後のバルーン（0 なら確かめない）
	}{
		{"CPUの上限を上げる", samples(3, 95, 60), nil, []string{"cpulimit"}, "CPU を変更しました", 2.5, 0},
		{"メモリを増やす", samples(3, 50, 95), nil, []string{"balloon"}, "メモリを変更しました", 0, 2560},
		{"メモリを減らす", samples(3, 50, 5), nil, []string{"balloon"}, "メモリを変更しました", 0, 1536},
		{"両方変える", samples(3, 95, 95), nil, []string{"cpulimit", "balloon"}, "CPU を変更しました、メモリを変更しました", 2.5, 2560},
		{"バルーンが目標に届かない", samples(3, 50, 95), func(c *fakeClient) { c.stuck = true }, []string{"balloon"}, "目標に届かず", 0, 2048},
		{"変更の API が失敗", samples(3, 95, 60), func(c *fakeClient) { c.failErr = errors.New("403 Permission check failed") }, []string{"cpulimit"}, "CPU の変更に失敗", 2, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &fakeRecorder{}
			client := newClient()
			if tt.setup != nil {
				tt.setup(client)
			}
			s := newScaler(client, fakeDB{targets: target, samples: tt.samples}, vm, false, rec)

			s.RunOnce(context.Background())

			if strings.Join(client.calls, ",") != strings.Join(tt.wantCalls, ",") {
				t.Errorf("呼んだ変更 = %v, want %v", client.calls, tt.wantCalls)
			}
			if len(*rec) != 1 || (*rec)[0].DryRun || !strings.Contains((*rec)[0].Message, tt.wantMsg) {
				t.Fatalf("記録 = %+v", *rec)
			}
			if tt.wantCPU != 0 && client.cfg.CPULimit != tt.wantCPU {
				t.Errorf("CPU の上限 = %v, want %v", client.cfg.CPULimit, tt.wantCPU)
			}
			if tt.wantMemMB != 0 && client.st.Balloon != tt.wantMemMB*mib {
				t.Errorf("バルーン = %d MB, want %d", client.st.Balloon/mib, tt.wantMemMB)
			}
		})
	}
}

func TestNameMismatchWarnsOnce(t *testing.T) {
	rec := &fakeRecorder{}
	vm := proxmox.VM{VMID: 101, Name: "app", Node: "pve1", Status: "running"}
	s := newScaler(newClient(), fakeDB{targets: []store.Target{{VMID: 101, Name: "old"}}, samples: samples(3, 95, 60)}, vm, false, rec)
	for range 3 {
		s.RunOnce(context.Background())
	}
	if len(*rec) != 1 {
		t.Errorf("名前が違う警告は1回だけのはず: %d 回", len(*rec))
	}
}

func TestSwapGrowth(t *testing.T) {
	vms := fakeVMs{{VMID: 101, Name: "db", Node: "pve1", Status: "running", SwapOut: 1000}}
	rec := &fakeRecorder{}
	client := newClient()
	s := New(config.Default().Autoscale, false, vms, fakeDB{targets: []store.Target{{VMID: 101, Name: "db"}}, samples: samples(3, 50, 80)},
		client, rec, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.verifyTimeout, s.verifyInterval = 10*time.Millisecond, time.Millisecond
	now := time.Unix(1_700_000_000, 0)
	s.now = func() time.Time { return now }

	// 上から順に実行する。swapOut はその回の VM のスワップの累計
	steps := []struct {
		name       string
		after      time.Duration
		swapOut    int64
		wantEvents int
	}{
		{"1回目は比べる相手がないので何もしない", 0, 1000, 0},
		{"スワップが増えたので増やす", 10 * time.Second, 5000, 1},
		{"クールダウン中は何もしない", 20 * time.Second, 9000, 1},
		{"クールダウン後、もう増えていなければ何もしない", 10 * time.Minute, 9000, 1},
	}
	for _, st := range steps {
		now = now.Add(st.after)
		vms[0].SwapOut = st.swapOut
		s.RunOnce(context.Background())
		if len(*rec) != st.wantEvents {
			t.Fatalf("%s: 記録の数 = %d, want %d: %+v", st.name, len(*rec), st.wantEvents, *rec)
		}
	}
	if !strings.Contains((*rec)[0].Message, "スワップが増えた") || !strings.Contains((*rec)[0].Message, "2048 → 2560 MB") {
		t.Errorf("記録 = %s", (*rec)[0].Message)
	}
}
