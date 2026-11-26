package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestEvents(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i, msg := range []string{"1件目", "2件目", "3件目"} {
		e, err := s.AddEvent(ctx, Event{Time: time.Unix(int64(i), 0), VMID: 101, Kind: "autoscale", Message: msg, DryRun: i == 0})
		if err != nil {
			t.Fatal(err)
		}
		if e.ID != int64(i+1) {
			t.Errorf("ID = %d, want %d", e.ID, i+1)
		}
	}

	tests := []struct {
		name      string
		limit     int
		wantMsgs  []string
		wantFirst bool // 最後の1件の dry_run
	}{
		{"新しい順に2件", 2, []string{"3件目", "2件目"}, false},
		{"全件", 10, []string{"3件目", "2件目", "1件目"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.Events(ctx, tt.limit)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.wantMsgs) {
				t.Fatalf("件数 = %d, want %d", len(got), len(tt.wantMsgs))
			}
			for i, m := range tt.wantMsgs {
				if got[i].Message != m {
					t.Errorf("%d件目 = %q, want %q", i, got[i].Message, m)
				}
			}
			if got[len(got)-1].DryRun != tt.wantFirst {
				t.Errorf("dry_run = %v", got[len(got)-1].DryRun)
			}
		})
	}
}

func TestDeleteEventsBefore(t *testing.T) {
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0)
	tests := []struct {
		name      string
		before    time.Time
		wantCount int64
		wantLeft  int
	}{
		{"どれも古くない", base, 0, 3},
		{"1件だけ古い", base.Add(time.Hour), 1, 2},
		{"すべて古い", base.Add(48 * time.Hour), 3, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			for _, d := range []time.Duration{0, 2 * time.Hour, 24 * time.Hour} {
				s.AddEvent(ctx, Event{Time: base.Add(d), VMID: 101, Kind: "autoscale", Message: "x"})
			}
			n, err := s.DeleteEventsBefore(ctx, tt.before)
			if err != nil {
				t.Fatal(err)
			}
			left, _ := s.Events(ctx, 10)
			if n != tt.wantCount || len(left) != tt.wantLeft {
				t.Errorf("削除 = %d, 残り = %d, want %d, %d", n, len(left), tt.wantCount, tt.wantLeft)
			}
		})
	}
}
