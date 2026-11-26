package proxmox

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newServer は1つのパスにだけ答える偽の Proxmox を立てる。届いたリクエストは got に入る。
func newServer(t *testing.T, method, path, body string, got *string) *HTTPClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method || r.URL.Path != path {
			http.Error(w, "not found: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
			return
		}
		if got != nil {
			r.ParseForm()
			*got = r.PostForm.Encode()
		}
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return NewHTTPClient(srv.URL, "id", "secret", false)
}

func TestGetVMConfig(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		want      VMConfig
		wantLimit float64
	}{
		{"数値で返る", `{"data":{"cores":4,"sockets":1,"memory":8192,"cpulimit":2}}`, VMConfig{Cores: 4, MemoryMB: 8192, CPULimit: 2}, 2},
		{"文字列で返る", `{"data":{"cores":"2","sockets":2,"memory":"current=4096","cpulimit":"1.5"}}`, VMConfig{Cores: 4, MemoryMB: 4096, CPULimit: 1.5}, 1.5},
		{"vcpus があればそちらの数", `{"data":{"cores":4,"sockets":1,"vcpus":2,"memory":4096}}`, VMConfig{Cores: 2, MemoryMB: 4096}, 2},
		{"cpulimit なしは上限なし", `{"data":{"cores":4,"memory":2048}}`, VMConfig{Cores: 4, MemoryMB: 2048}, 4},
		{"省略されたら Proxmox の初期値", `{"data":{}}`, VMConfig{Cores: 1, MemoryMB: 512}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newServer(t, "GET", "/api2/json/nodes/pve1/qemu/101/config", tt.body, nil)
			got, err := c.GetVMConfig(context.Background(), "pve1", 101)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
			if got.CurrentCPULimit() != tt.wantLimit {
				t.Errorf("CurrentCPULimit = %v, want %v", got.CurrentCPULimit(), tt.wantLimit)
			}
		})
	}
}

func TestGetVMStatus(t *testing.T) {
	const gib = 1 << 30
	tests := []struct {
		name      string
		body      string
		wantLimit int64
	}{
		{"バルーンで減らしている", `{"data":{"status":"running","mem":500,"maxmem":4294967296,"balloon":1073741824,"ballooninfo":{"mem_swapped_out":1703936}}}`, 1 * gib},
		{"バルーンなし", `{"data":{"status":"running","mem":500,"maxmem":4294967296}}`, 4 * gib},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newServer(t, "GET", "/api2/json/nodes/pve1/qemu/101/status/current", tt.body, nil)
			got, err := c.GetVMStatus(context.Background(), "pve1", 101)
			if err != nil {
				t.Fatal(err)
			}
			if got.MemLimit() != tt.wantLimit || got.Status != "running" || (tt.wantLimit == gib && got.SwapOut != 1703936) {
				t.Errorf("got %+v, MemLimit = %d", got, got.MemLimit())
			}
		})
	}
}

func TestSetBalloon(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantSent string
		wantErr  string
	}{
		{"成功は空で返る", `{"data":""}`, "command=balloon+1536", ""},
		{"モニターのエラー", `{"data":"Error: No balloon device has been activated"}`, "command=balloon+1536", "No balloon device"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sent string
			c := newServer(t, "POST", "/api2/json/nodes/pve1/qemu/101/monitor", tt.body, &sent)
			err := c.SetBalloon(context.Background(), "pve1", 101, 1536)
			if sent != tt.wantSent {
				t.Errorf("送った内容 = %q, want %q", sent, tt.wantSent)
			}
			if tt.wantErr == "" && err != nil {
				t.Fatalf("想定外のエラー: %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("エラーに %q が含まれるはず: %v", tt.wantErr, err)
			}
		})
	}
}

func TestSetCPULimit(t *testing.T) {
	tests := []struct {
		name     string
		cores    float64
		wantSent string
	}{
		{"整数", 2, "cpulimit=2"},
		{"小数", 2.5, "cpulimit=2.5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sent string
			c := newServer(t, "PUT", "/api2/json/nodes/pve1/qemu/101/config", `{"data":null}`, &sent)
			if err := c.SetCPULimit(context.Background(), "pve1", 101, tt.cores); err != nil {
				t.Fatal(err)
			}
			if sent != tt.wantSent {
				t.Errorf("送った内容 = %q, want %q", sent, tt.wantSent)
			}
		})
	}
}
