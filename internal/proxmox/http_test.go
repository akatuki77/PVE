package proxmox

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestListVMs(t *testing.T) {
	tests := []struct {
		name    string
		apiPath string // NewHTTPClient に渡す URL の末尾
		status  int
		body    string
		want    []VM
		wantErr string
	}{
		{
			name:   "qemuだけを返しタグを分ける",
			status: http.StatusOK,
			body: `{"data":[
				{"type":"qemu","vmid":101,"name":"db","node":"pve1","status":"running","cpu":0.5,"maxcpu":2,"mem":100,"maxmem":200,"tags":"demo;db"},
				{"type":"lxc","vmid":200,"name":"ct","node":"pve1","status":"running"},
				{"type":"qemu","vmid":9000,"name":"tpl","node":"pve1","status":"stopped","template":1}
			]}`,
			want: []VM{{VMID: 101, Name: "db", Node: "pve1", Status: "running", CPU: 0.5, MaxCPU: 2, Mem: 100, MaxMem: 200, Tags: []string{"demo", "db"}}},
		},
		{
			name:    "URLに /api2/json が付いていても動く",
			apiPath: "/api2/json/",
			status:  http.StatusOK,
			body:    `{"data":[]}`,
			want:    []VM{},
		},
		{
			name:    "認証エラー",
			status:  http.StatusUnauthorized,
			body:    "authentication failure",
			wantErr: "401",
		},
		{
			name:    "JSONが壊れている",
			status:  http.StatusOK,
			body:    `{"data":`,
			wantErr: "解析に失敗",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.RequestURI() != "/api2/json/cluster/resources?type=vm" {
					t.Errorf("パスが違う: %s", r.URL.RequestURI())
				}
				if r.Header.Get("Authorization") != "PVEAPIToken=id@pve!t=secret" {
					t.Errorf("認証ヘッダーが違う")
				}
				w.WriteHeader(tt.status)
				io.WriteString(w, tt.body)
			}))
			defer srv.Close()

			c := NewHTTPClient(srv.URL+tt.apiPath, "id@pve!t", "secret", false)
			got, err := c.ListVMs(context.Background())
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("エラーに %q が含まれるはず: %v", tt.wantErr, err)
				}
				if strings.Contains(err.Error(), "secret") {
					t.Fatal("エラーにトークンが含まれている")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v\nwant %+v", got, tt.want)
			}
		})
	}
}
