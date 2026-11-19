package monitor

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/akatuki77/proxconductor/internal/proxmox"
	"github.com/akatuki77/proxconductor/internal/store"
)

type fakeClient struct {
	vms []proxmox.VM
	err error
}

func (f fakeClient) ListVMs(context.Context) ([]proxmox.VM, error) { return f.vms, f.err }

type fakeStore struct {
	saved   []store.Metric
	deleted int
}

func (f *fakeStore) SaveMetrics(_ context.Context, ms []store.Metric) error {
	f.saved = append(f.saved, ms...)
	return nil
}

func (f *fakeStore) DeleteMetricsBefore(context.Context, time.Time) (int64, error) {
	f.deleted++
	return 0, nil
}

func TestCollectOnce(t *testing.T) {
	tests := []struct {
		name       string
		client     fakeClient
		wantSaved  int
		wantVMs    int
		wantErr    bool
		wantUpdate bool
	}{
		{
			name: "起動中のVMだけ保存する",
			client: fakeClient{vms: []proxmox.VM{
				{VMID: 101, Status: "running", CPU: 0.5},
				{VMID: 102, Status: "stopped"},
			}},
			wantSaved: 1, wantVMs: 2, wantUpdate: true,
		},
		{
			name:    "取得に失敗したらエラーを覚える",
			client:  fakeClient{err: errors.New("401 authentication failure")},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &fakeStore{}
			var notified []Snapshot
			c := New(tt.client, st, time.Second, time.Hour, slog.New(slog.NewTextHandler(io.Discard, nil)),
				func(s Snapshot) { notified = append(notified, s) })
			c.CollectOnce(context.Background())
			if len(notified) != 1 {
				t.Fatalf("知らせの回数 = %d, want 1", len(notified))
			}
			if (notified[0].Err != "") != tt.wantErr || len(notified[0].VMs) != tt.wantVMs {
				t.Errorf("知らせの中身 = %+v", notified[0])
			}

			got := c.Latest()
			if len(st.saved) != tt.wantSaved {
				t.Errorf("保存件数 = %d, want %d", len(st.saved), tt.wantSaved)
			}
			if len(got.VMs) != tt.wantVMs {
				t.Errorf("VM数 = %d, want %d", len(got.VMs), tt.wantVMs)
			}
			if (got.Err != "") != tt.wantErr {
				t.Errorf("Err = %q", got.Err)
			}
			if got.UpdatedAt.IsZero() == tt.wantUpdate {
				t.Errorf("UpdatedAt = %v", got.UpdatedAt)
			}
		})
	}
}

func TestErrorKeepsLastVMs(t *testing.T) {
	st := &fakeStore{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	c := New(fakeClient{vms: []proxmox.VM{{VMID: 101, Status: "running"}}}, st, time.Second, time.Hour, logger, nil)
	c.CollectOnce(context.Background())

	c.client = fakeClient{err: errors.New("timeout")}
	c.CollectOnce(context.Background())

	got := c.Latest()
	if len(got.VMs) != 1 || got.Err == "" {
		t.Errorf("前回のVM一覧を残し、エラーも記録するはず: %+v", got)
	}
}
