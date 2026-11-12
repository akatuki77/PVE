package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOpen(t *testing.T) {
	tests := []struct {
		name    string
		path    func(dir string) string
		wantErr bool
	}{
		{"新しいファイル", func(dir string) string { return filepath.Join(dir, "test.db") }, false},
		{"フォルダがなくても作る", func(dir string) string { return filepath.Join(dir, "sub", "test.db") }, false},
		{"フォルダを作れない場所はエラー", func(dir string) string {
			file := filepath.Join(dir, "file")
			os.WriteFile(file, nil, 0o644)
			return filepath.Join(file, "test.db")
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			s, err := Open(ctx, tt.path(t.TempDir()))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			defer s.Close()
			if err := s.PingContext(ctx); err != nil {
				t.Errorf("Ping に失敗: %v", err)
			}
		})
	}
}
