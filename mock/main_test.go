package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
