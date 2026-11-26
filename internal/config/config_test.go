package config

import (
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string // 空ならエラーなし
		check   func(t *testing.T, c Config)
	}{
		{
			name: "空なら初期値",
			yaml: "",
			check: func(t *testing.T, c Config) {
				if !c.DryRun {
					t.Error("dry_run の初期値は true のはず")
				}
				if c.Autoscale.Cooldown != 5*time.Minute {
					t.Errorf("cooldown = %v", c.Autoscale.Cooldown)
				}
			},
		},
		{
			name: "書いた項目だけ上書きされる",
			yaml: "dry_run: false\nautoscale:\n  cooldown: 90s\n",
			check: func(t *testing.T, c Config) {
				if c.DryRun {
					t.Error("dry_run は false のはず")
				}
				if c.Autoscale.Cooldown != 90*time.Second || c.Autoscale.Interval != 30*time.Second {
					t.Errorf("cooldown = %v, interval = %v", c.Autoscale.Cooldown, c.Autoscale.Interval)
				}
			},
		},
		{
			name: "VMごとの上書き",
			yaml: `
autoscale:
  overrides:
    - vmid: 101
      cpu: {min: 2, max: 8, step: 1, up_threshold: 70, down_threshold: 10}
      memory: {min_mb: 2048, max_mb: 16384, step_mb: 1024, margin_mb: 256, up_threshold: 80, down_threshold: 20}
`,
			check: func(t *testing.T, c Config) {
				if got := c.Autoscale.PolicyFor(101).CPU.Max; got != 8 {
					t.Errorf("VM 101 の cpu.max = %v, want 8", got)
				}
				if got := c.Autoscale.PolicyFor(102).CPU.Max; got != 4 {
					t.Errorf("VM 102 は初期値のはず: cpu.max = %v", got)
				}
			},
		},
		{
			name:    "YAMLが壊れている",
			yaml:    "autoscale: [",
			wantErr: "解析に失敗",
		},
		{
			name:    "クールダウン0はエラー",
			yaml:    "autoscale:\n  cooldown: 0s\n",
			wantErr: "cooldown",
		},
		{
			name:    "上限が下限より小さい",
			yaml:    "autoscale:\n  default:\n    cpu: {min: 4, max: 2, step: 1, up_threshold: 80, down_threshold: 20}\n",
			wantErr: "cpu の min",
		},
		{
			name:    "CPUの1回に変える量が0",
			yaml:    "autoscale:\n  default:\n    cpu: {min: 1, max: 4, step: 0, up_threshold: 80, down_threshold: 20}\n",
			wantErr: "step",
		},
		{
			name:    "しきい値の大小が逆",
			yaml:    "autoscale:\n  default:\n    memory: {min_mb: 1024, max_mb: 2048, step_mb: 256, up_threshold: 20, down_threshold: 80}\n",
			wantErr: "memory のしきい値",
		},
		{
			name: "上書きのVMが重複",
			yaml: `
autoscale:
  overrides:
    - {vmid: 101, cpu: {min: 1, max: 2, step: 1, up_threshold: 80, down_threshold: 20}, memory: {min_mb: 512, max_mb: 1024, step_mb: 256, up_threshold: 80, down_threshold: 20}}
    - {vmid: 101, cpu: {min: 1, max: 2, step: 1, up_threshold: 80, down_threshold: 20}, memory: {min_mb: 512, max_mb: 1024, step_mb: 256, up_threshold: 80, down_threshold: 20}}
`,
			wantErr: "重複",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := Parse([]byte(tt.yaml))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("エラーに %q が含まれるはず: %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("想定外のエラー: %v", err)
			}
			tt.check(t, c)
		})
	}
}

func TestExampleFiles(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"本番用の例", "../../deploy/config.example.yaml"},
		{"開発用", "../../deploy/config.dev.yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Load(tt.path); err != nil {
				t.Fatalf("読めない: %v", err)
			}
		})
	}
}
