package store

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestMetrics(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	base := time.Unix(1_700_000_000, 0)
	err = s.SaveMetrics(ctx, []Metric{
		{VMID: 101, Time: base, CPU: 0.1, CPUs: 2, MemUsed: 1, MemMax: 10},
		{VMID: 101, Time: base.Add(time.Minute), CPU: 0.2, CPUs: 2, MemUsed: 2, MemMax: 10},
		{VMID: 102, Time: base.Add(time.Minute), CPU: 0.9, CPUs: 1, MemUsed: 5, MemMax: 10},
	})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		vmid    int
		since   time.Time
		wantCPU []float64
	}{
		{"全期間", 101, base, []float64{0.1, 0.2}},
		{"期間で絞る", 101, base.Add(30 * time.Second), []float64{0.2}},
		{"VMで絞る", 102, base, []float64{0.9}},
		{"データなし", 999, base, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms, err := s.Metrics(ctx, tt.vmid, tt.since)
			if err != nil {
				t.Fatal(err)
			}
			var got []float64
			for _, m := range ms {
				got = append(got, m.CPU)
			}
			if !reflect.DeepEqual(got, tt.wantCPU) {
				t.Errorf("got %v, want %v", got, tt.wantCPU)
			}
		})
	}
}

func TestDeleteMetricsBefore(t *testing.T) {
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0)
	tests := []struct {
		name      string
		before    time.Time
		wantCount int64
		wantLeft  int
	}{
		{"どれも古くない", base, 0, 3},
		{"1件だけ古い", base.Add(30 * time.Second), 1, 2},
		{"すべて古い", base.Add(time.Hour), 3, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			s.SaveMetrics(ctx, []Metric{
				{VMID: 101, Time: base},
				{VMID: 101, Time: base.Add(time.Minute)},
				{VMID: 101, Time: base.Add(2 * time.Minute)},
			})

			n, err := s.DeleteMetricsBefore(ctx, tt.before)
			if err != nil {
				t.Fatal(err)
			}
			if n != tt.wantCount {
				t.Errorf("削除件数 = %d, want %d", n, tt.wantCount)
			}
			left, _ := s.Metrics(ctx, 101, time.Unix(0, 0))
			if len(left) != tt.wantLeft {
				t.Errorf("残り = %d, want %d", len(left), tt.wantLeft)
			}
		})
	}
}
