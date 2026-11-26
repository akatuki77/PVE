// Package config は YAML の設定ファイルを読む。
package config

import (
	"errors"
	"fmt"
	"os"
	"time"

	"go.yaml.in/yaml/v3"
)

// Config は設定ファイルの中身。
type Config struct {
	DryRun    bool      `yaml:"dry_run"` // true なら VM を変更しない
	Autoscale Autoscale `yaml:"autoscale"`
}

// Autoscale はオートスケールの設定。
type Autoscale struct {
	Interval  time.Duration `yaml:"interval"` // 判定の間隔
	Cooldown  time.Duration `yaml:"cooldown"` // 同じ VM を続けて変更しない時間
	Window    time.Duration `yaml:"window"`   // 平均をとる期間
	Default   Policy        `yaml:"default"`
	Overrides []Override    `yaml:"overrides"`
}

// Policy は1台分のスケールの決まり。
type Policy struct {
	CPU    CPUPolicy    `yaml:"cpu"`
	Memory MemoryPolicy `yaml:"memory"`
}

// CPUPolicy は CPU の上限（cpulimit、コア数）の決まり。しきい値は %。
type CPUPolicy struct {
	Min           float64 `yaml:"min"`
	Max           float64 `yaml:"max"` // VM のコア数を超える分は使わない
	Step          float64 `yaml:"step"`
	UpThreshold   float64 `yaml:"up_threshold"`
	DownThreshold float64 `yaml:"down_threshold"`
}

// MemoryPolicy はメモリ（バルーン）の決まり。しきい値は %。
type MemoryPolicy struct {
	MinMB         int     `yaml:"min_mb"`
	MaxMB         int     `yaml:"max_mb"` // VM のメモリの設定値を超える分は使わない
	StepMB        int     `yaml:"step_mb"`
	MarginMB      int     `yaml:"margin_mb"` // 減らすとき、使っている量にこれを足した量より下げない
	UpThreshold   float64 `yaml:"up_threshold"`
	DownThreshold float64 `yaml:"down_threshold"`
}

// Override は特定の VM だけ決まりを変える。
type Override struct {
	VMID   int `yaml:"vmid"`
	Policy `yaml:",inline"`
}

// Default は設定ファイルがないときの値。
func Default() Config {
	return Config{
		DryRun: true,
		Autoscale: Autoscale{
			Interval: 30 * time.Second,
			Cooldown: 5 * time.Minute,
			Window:   2 * time.Minute,
			Default: Policy{
				CPU:    CPUPolicy{Min: 1, Max: 4, Step: 0.5, UpThreshold: 80, DownThreshold: 20},
				Memory: MemoryPolicy{MinMB: 1024, MaxMB: 8192, StepMB: 512, MarginMB: 256, UpThreshold: 85, DownThreshold: 30},
			},
		},
	}
}

// Load は設定ファイルを読む。path が空なら初期値を返す。
func Load(path string) (Config, error) {
	if path == "" {
		return Default(), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("設定ファイルの読み込みに失敗: %w", err)
	}
	return Parse(data)
}

// Parse は YAML を読み、初期値で補って検証する。
func Parse(data []byte) (Config, error) {
	cfg := Default()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("設定ファイルの解析に失敗: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// PolicyFor は VM に使う決まりを返す。
func (a Autoscale) PolicyFor(vmid int) Policy {
	for _, o := range a.Overrides {
		if o.VMID == vmid {
			return o.Policy
		}
	}
	return a.Default
}

func (c Config) validate() error {
	var errs []error
	a := c.Autoscale
	if a.Interval <= 0 || a.Cooldown <= 0 || a.Window <= 0 {
		errs = append(errs, errors.New("autoscale の interval・cooldown・window は 0 より大きくしてください"))
	}
	if err := a.Default.validate("default"); err != nil {
		errs = append(errs, err)
	}
	seen := map[int]bool{}
	for _, o := range a.Overrides {
		if o.VMID <= 0 {
			errs = append(errs, errors.New("overrides の vmid が正しくありません"))
			continue
		}
		if seen[o.VMID] {
			errs = append(errs, fmt.Errorf("overrides に VM %d が重複しています", o.VMID))
		}
		seen[o.VMID] = true
		if err := o.Policy.validate(fmt.Sprintf("VM %d", o.VMID)); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("設定が正しくありません: %w", errors.Join(errs...))
	}
	return nil
}

func (p Policy) validate(name string) error {
	var errs []error
	if p.CPU.Min <= 0 || p.CPU.Max < p.CPU.Min || p.CPU.Step <= 0 {
		errs = append(errs, fmt.Errorf("%s: cpu の min・step は 0 より大きく、max は min 以上にしてください", name))
	}
	if !validThresholds(p.CPU.DownThreshold, p.CPU.UpThreshold) {
		errs = append(errs, fmt.Errorf("%s: cpu のしきい値は 0〜100 で down < up にしてください", name))
	}
	if p.Memory.MinMB < 1 || p.Memory.MaxMB < p.Memory.MinMB || p.Memory.StepMB < 1 || p.Memory.MarginMB < 0 {
		errs = append(errs, fmt.Errorf("%s: memory の min_mb・max_mb・step_mb・margin_mb が正しくありません", name))
	}
	if !validThresholds(p.Memory.DownThreshold, p.Memory.UpThreshold) {
		errs = append(errs, fmt.Errorf("%s: memory のしきい値は 0〜100 で down < up にしてください", name))
	}
	return errors.Join(errs...)
}

func validThresholds(down, up float64) bool {
	return down >= 0 && up <= 100 && down < up
}
