package store

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestTargets(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Unix(1_700_000_000, 0)

	// 上から順に実行し、そのたびに一覧を確かめる
	steps := []struct {
		name string
		do   func() error
		want []Target
	}{
		{"最初は空", func() error { return nil }, nil},
		{"追加", func() error { return s.AddTarget(ctx, 102, "app", now) }, []Target{{102, "app", now}}},
		{"もう1台", func() error { return s.AddTarget(ctx, 101, "db", now) }, []Target{{101, "db", now}, {102, "app", now}}},
		{"同じVMは名前を更新", func() error { return s.AddTarget(ctx, 102, "app2", now) }, []Target{{101, "db", now}, {102, "app2", now}}},
		{"外す", func() error { return s.RemoveTarget(ctx, 101) }, []Target{{102, "app2", now}}},
		{"ないVMを外してもエラーにしない", func() error { return s.RemoveTarget(ctx, 999) }, []Target{{102, "app2", now}}},
	}
	for _, st := range steps {
		if err := st.do(); err != nil {
			t.Fatalf("%s: %v", st.name, err)
		}
		got, err := s.Targets(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, st.want) {
			t.Errorf("%s: got %+v, want %+v", st.name, got, st.want)
		}
	}
}
