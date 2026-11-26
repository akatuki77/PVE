package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMockAPI(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		auth      bool
		wantCode  int
		wantCount int // data が配列のときの件数。-1 なら確かめない
	}{
		{"トークンなしは401", "/api2/json/cluster/resources?type=vm", false, http.StatusUnauthorized, -1},
		{"VM一覧は3台", "/api2/json/cluster/resources?type=vm", true, http.StatusOK, 3},
		{"VM以外の種類は0件", "/api2/json/cluster/resources?type=storage", true, http.StatusOK, 0},
		{"VMの状態", "/api2/json/nodes/pve1/qemu/101/status/current", true, http.StatusOK, -1},
		{"ノードが違うと500", "/api2/json/nodes/pve2/qemu/101/status/current", true, http.StatusInternalServerError, -1},
		{"VMの設定", "/api2/json/nodes/pve1/qemu/101/config", true, http.StatusOK, -1},
	}
	h := newCluster().handler()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			if tt.auth {
				req.Header.Set("Authorization", "PVEAPIToken=mock@pve!dev=mock")
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.wantCode {
				t.Fatalf("code = %d, want %d", rec.Code, tt.wantCode)
			}
			if tt.wantCount < 0 {
				return
			}
			var body struct {
				Data []map[string]any `json:"data"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if len(body.Data) != tt.wantCount {
				t.Errorf("件数 = %d, want %d", len(body.Data), tt.wantCount)
			}
		})
	}
}

func TestMockChanges(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		form      string
		wantCode  int
		wantLimit float64
		wantMB    int
	}{
		{"cpulimit を変える", "/api2/json/nodes/pve1/qemu/101/config", "cpulimit=3", http.StatusOK, 3, 2048},
		{"バルーンを変える", "/api2/json/nodes/pve1/qemu/101/monitor", "command=balloon+1536", http.StatusOK, 2, 1536},
		{"バルーンは設定値を超えない", "/api2/json/nodes/pve1/qemu/101/monitor", "command=balloon+9999", http.StatusOK, 2, 4096},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newCluster()
			method := "POST"
			if strings.HasSuffix(tt.path, "/config") {
				method = "PUT"
			}
			req := httptest.NewRequest(method, tt.path, strings.NewReader(tt.form))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Authorization", "PVEAPIToken=mock@pve!dev=mock")
			rec := httptest.NewRecorder()
			c.handler().ServeHTTP(rec, req)
			if rec.Code != tt.wantCode {
				t.Fatalf("code = %d (%s)", rec.Code, rec.Body)
			}
			v := c.vms[0]
			if v.CPULimit != tt.wantLimit || v.BalloonMB != tt.wantMB {
				t.Errorf("cpulimit = %v, balloon = %d", v.CPULimit, v.BalloonMB)
			}
		})
	}
}
