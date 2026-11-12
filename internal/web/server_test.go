package web

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

type fakeDB struct{ err error }

func (f fakeDB) PingContext(context.Context) error { return f.err }

var testFiles = fstest.MapFS{
	"index.html":  {Data: []byte("<html>app</html>")},
	"assets/a.js": {Data: []byte("js")},
}

func TestRoutes(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		dbErr    error
		wantCode int
		wantBody string
	}{
		{"正常", "GET", "/api/healthz", nil, http.StatusOK, `{"status":"ok","db":"ok"}`},
		{"DBが使えない", "GET", "/api/healthz", errors.New("closed"), http.StatusServiceUnavailable, `"db":"error"`},
		{"GET以外は405", "POST", "/api/healthz", nil, http.StatusMethodNotAllowed, ""},
		{"ないAPIは404", "GET", "/api/nothing", nil, http.StatusNotFound, ""},
		{"画面のトップ", "GET", "/", nil, http.StatusOK, "app"},
		{"画面のファイル", "GET", "/assets/a.js", nil, http.StatusOK, "js"},
		{"画面内のパスは index.html", "GET", "/vms/101", nil, http.StatusOK, "app"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewServer(slog.New(slog.NewTextHandler(io.Discard, nil)), fakeDB{err: tt.dbErr}, testFiles)
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
			if rec.Code != tt.wantCode {
				t.Errorf("code = %d, want %d", rec.Code, tt.wantCode)
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want %q を含む", rec.Body.String(), tt.wantBody)
			}
		})
	}
}
