package web

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/akatuki77/proxconductor/internal/monitor"
	"github.com/akatuki77/proxconductor/internal/proxmox"
	"github.com/akatuki77/proxconductor/internal/store"
)

type fakeDB struct {
	pingErr    error
	metricsErr error
	targetsErr error
	targets    map[int]string // VMID → 選んだときの名前
}

func (f fakeDB) AddEvent(_ context.Context, e store.Event) (store.Event, error) {
	e.ID = 1
	return e, nil
}

func (f fakeDB) DeleteEventsBefore(context.Context, time.Time) (int64, error) { return 0, nil }

func (f fakeDB) Events(context.Context, int) ([]store.Event, error) {
	return []store.Event{{ID: 1, VMID: 101, Kind: "autoscale", Message: "CPU を増やす", DryRun: true}}, nil
}

func (f fakeDB) Targets(context.Context) ([]store.Target, error) {
	var ts []store.Target
	for id, name := range f.targets {
		ts = append(ts, store.Target{VMID: id, Name: name})
	}
	return ts, f.targetsErr
}

func (f fakeDB) AddTarget(_ context.Context, vmid int, name string, _ time.Time) error {
	if f.targets != nil {
		f.targets[vmid] = name
	}
	return nil
}

func (f fakeDB) RemoveTarget(_ context.Context, vmid int) error {
	delete(f.targets, vmid)
	return nil
}

func (f fakeDB) PingContext(context.Context) error { return f.pingErr }

func (f fakeDB) Metrics(_ context.Context, vmid int, _ time.Time) ([]store.Metric, error) {
	if f.metricsErr != nil {
		return nil, f.metricsErr
	}
	if vmid != 101 {
		return nil, nil
	}
	return []store.Metric{{VMID: 101, Time: time.Unix(1, 0), CPU: 0.5, CPUs: 2, MemUsed: 1, MemMax: 4}}, nil
}

type fakeVMs struct{ snap monitor.Snapshot }

func (f fakeVMs) Latest() monitor.Snapshot { return f.snap }

var testFiles = fstest.MapFS{
	"index.html":  {Data: []byte("<html>app</html>")},
	"assets/a.js": {Data: []byte("js")},
}

var oneVM = monitor.Snapshot{
	VMs:       []proxmox.VM{{VMID: 101, Name: "db", Status: "running"}},
	UpdatedAt: time.Unix(1, 0),
}

func TestRoutes(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		db       fakeDB
		snap     monitor.Snapshot
		wantCode int
		wantBody string
	}{
		{"healthz 正常", "GET", "/api/healthz", fakeDB{}, oneVM, 200, `{"status":"ok","db":"ok"}`},
		{"healthz DBが使えない", "GET", "/api/healthz", fakeDB{pingErr: errors.New("closed")}, oneVM, 503, `"db":"error"`},
		{"GET以外は405", "POST", "/api/healthz", fakeDB{}, oneVM, 405, ""},
		{"ないAPIは404", "GET", "/api/nothing", fakeDB{}, oneVM, 404, ""},
		{"VM一覧", "GET", "/api/vms", fakeDB{}, oneVM, 200, `"name":"db"`},
		{"取得前のVM一覧は空", "GET", "/api/vms", fakeDB{}, monitor.Snapshot{}, 200, `{"vms":[],"updated_at":null,"error":""}`},
		{"取得エラーを返す", "GET", "/api/vms", fakeDB{}, monitor.Snapshot{Err: "401"}, 200, `"error":"401"`},
		{"使用量の推移", "GET", "/api/vms/101/metrics", fakeDB{}, oneVM, 200, `"cpu":50`},
		{"期間の指定", "GET", "/api/vms/101/metrics?range=24h", fakeDB{}, oneVM, 200, `"range":"24h"`},
		{"データなしは空の配列", "GET", "/api/vms/102/metrics", fakeDB{}, oneVM, 200, `"points":[]`},
		{"期間が不正", "GET", "/api/vms/101/metrics?range=9y", fakeDB{}, oneVM, 400, "range"},
		{"VMIDが不正", "GET", "/api/vms/abc/metrics", fakeDB{}, oneVM, 400, "VMID"},
		{"DBエラー", "GET", "/api/vms/101/metrics", fakeDB{metricsErr: errors.New("x")}, oneVM, 500, "失敗"},
		{"対象外のVM", "GET", "/api/vms", fakeDB{}, oneVM, 200, `"autoscale":false,"autoscale_mismatch":false`},
		{"対象のVM", "GET", "/api/vms", fakeDB{targets: map[int]string{101: "db"}}, oneVM, 200, `"autoscale":true,"autoscale_mismatch":false`},
		{"選んだときと名前が違う", "GET", "/api/vms", fakeDB{targets: map[int]string{101: "old"}}, oneVM, 200, `"autoscale":true,"autoscale_mismatch":true`},
		{"対象の読み込みエラー", "GET", "/api/vms", fakeDB{targetsErr: errors.New("x")}, oneVM, 500, "失敗"},
		{"対象にする", "PUT", "/api/vms/101/autoscale", fakeDB{}, oneVM, 204, ""},
		{"いないVMは対象にできない", "PUT", "/api/vms/999/autoscale", fakeDB{}, oneVM, 404, "見つかりません"},
		{"対象にするVMIDが不正", "PUT", "/api/vms/abc/autoscale", fakeDB{}, oneVM, 400, "VMID"},
		{"対象から外す", "DELETE", "/api/vms/101/autoscale", fakeDB{}, oneVM, 204, ""},
		{"記録の一覧", "GET", "/api/events", fakeDB{}, oneVM, 200, `"message":"CPU を増やす"`},
		{"記録の件数が不正", "GET", "/api/events?limit=0", fakeDB{}, oneVM, 400, "limit"},
		{"画面のトップ", "GET", "/", fakeDB{}, oneVM, 200, "app"},
		{"画面のファイル", "GET", "/assets/a.js", fakeDB{}, oneVM, 200, "js"},
		{"画面内のパスは index.html", "GET", "/vms/101", fakeDB{}, oneVM, 200, "app"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewServer(Options{
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
				DB:     tt.db,
				VMs:    fakeVMs{snap: tt.snap},
				Broker: NewBroker(),
				Static: testFiles,
			})
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
			if rec.Code != tt.wantCode {
				t.Errorf("code = %d, want %d (%s)", rec.Code, tt.wantCode, rec.Body)
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want %q を含む", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestTargetToggle(t *testing.T) {
	db := fakeDB{targets: map[int]string{}}
	b := NewBroker()
	ch := b.subscribe()
	s := NewServer(Options{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:     db,
		VMs:    fakeVMs{snap: oneVM},
		Broker: b,
	})
	h := s.Handler()

	steps := []struct {
		method   string
		wantName string // 対象の一覧での VM 101 の名前（空なら対象外）
		wantSent string // 画面に送られた内容に含まれるはずの文字
	}{
		{"PUT", "db", `"autoscale":true`},
		{"DELETE", "", `"autoscale":false`},
	}
	for _, st := range steps {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(st.method, "/api/vms/101/autoscale", nil))
		if got := db.targets[101]; got != st.wantName {
			t.Errorf("%s 後の名前 = %q, want %q", st.method, got, st.wantName)
		}
		select {
		case msg := <-ch:
			if !strings.Contains(string(msg.Data), st.wantSent) {
				t.Errorf("%s 後に送られた内容 = %s", st.method, msg.Data)
			}
		default:
			t.Errorf("%s 後に画面へ送られていない", st.method)
		}
	}
}

func TestDownsample(t *testing.T) {
	mk := func(n int) []store.Metric {
		ms := make([]store.Metric, n)
		for i := range ms {
			ms[i] = store.Metric{Time: time.Unix(int64(i), 0), CPU: 0.5, MemUsed: 1, MemMax: 2}
		}
		return ms
	}
	tests := []struct {
		name  string
		n     int
		limit int
		want  int
	}{
		{"空", 0, 10, 0},
		{"上限以下はそのまま", 5, 10, 5},
		{"上限を超えたらまとめる", 100, 10, 10},
		{"割り切れない場合", 25, 10, 9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := downsample(mk(tt.n), tt.limit)
			if len(got) != tt.want {
				t.Errorf("点の数 = %d, want %d", len(got), tt.want)
			}
			for _, p := range got {
				if p.CPU != 50 || p.Mem != 50 {
					t.Errorf("平均が違う: %+v", p)
				}
			}
		})
	}
}
