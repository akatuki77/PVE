package autoscale

import (
	"testing"

	"github.com/akatuki77/proxconductor/internal/config"
	"github.com/akatuki77/proxconductor/internal/store"
)

var policy = config.Policy{
	CPU:    config.CPUPolicy{Min: 1, Max: 4, Step: 0.5, UpThreshold: 80, DownThreshold: 20},
	Memory: config.MemoryPolicy{MinMB: 1024, MaxMB: 4096, StepMB: 512, MarginMB: 256, UpThreshold: 85, DownThreshold: 30},
}

// samples は、上限 2 コア・使えるメモリ 1000 に対する使用率（%）の使用量を n 件作る。
// CPU は 4 コアの VM の値として作る（上限 2 コアの 100% は全体の 50%）。
func samples(n int, cpuPct, memPct float64) []store.Metric {
	ms := make([]store.Metric, n)
	for i := range ms {
		ms[i] = store.Metric{CPU: cpuPct / 100 * 2 / 4, CPUs: 4, MemUsed: int64(memPct * 10), MemMax: 1000}
	}
	return ms
}

// samplesAt は、上限 limit コア・全 cores コアの VM で、上限に対する CPU 使用率（%）の使用量を n 件作る。
func samplesAt(n int, cpuPct, limit float64, cores int) []store.Metric {
	ms := make([]store.Metric, n)
	for i := range ms {
		ms[i] = store.Metric{CPU: cpuPct / 100 * limit / float64(cores), CPUs: cores, MemUsed: 600, MemMax: 1000}
	}
	return ms
}

// swapping はスワップが増えている State を作る。
func swapping(memMB, usedMB int) State {
	s := st(memMB, usedMB)
	s.SwapGrowing = true
	return s
}

// st は CPU の上限 2 コアの State を作る。
func st(memMB, usedMB int) State {
	return State{CPULimit: 2, Cores: 4, MemoryMB: memMB, MaxMemoryMB: 4096, UsedMB: usedMB}
}

func TestDecide(t *testing.T) {
	tests := []struct {
		name       string
		state      State
		samples    []store.Metric
		wantCPU    float64
		wantMemory int
	}{
		{"件数が足りないと何もしない", st(2048, 500), samples(2, 95, 95), 0, 0},
		{"どちらも中間なら何もしない", st(2048, 500), samples(3, 50, 60), 0, 0},
		{"CPUが高いと上限を step 分上げる", st(2048, 500), samples(3, 90, 60), 2.5, 0},
		{"CPUが低いと上限を step 分下げる", st(2048, 500), samples(3, 5, 60), 1.5, 0},
		{"CPUの上限はコア数で止まる", State{CPULimit: 4, Cores: 4, MemoryMB: 2048, MaxMemoryMB: 4096}, samplesAt(3, 95, 4, 4), 0, 0},
		{"CPUの上限は設定の max でも止まる", State{CPULimit: 4, Cores: 8, MemoryMB: 2048, MaxMemoryMB: 4096}, samplesAt(3, 95, 4, 8), 0, 0},
		{"CPUの上限は下限で止まる", State{CPULimit: 1, Cores: 4, MemoryMB: 2048, MaxMemoryMB: 4096}, samplesAt(3, 1, 1, 4), 0, 0},
		{"上限 3 から上げる", State{CPULimit: 3, Cores: 4, MemoryMB: 2048, MaxMemoryMB: 4096}, samplesAt(3, 95, 3, 4), 3.5, 0},
		{"メモリが高いと step 分増やす", st(2048, 500), samples(3, 50, 90), 0, 2560},
		{"メモリの増加は上限で切る", st(3800, 500), samples(3, 50, 90), 0, 4096},
		{"メモリの増加は設定値でも切る", State{CPULimit: 2, Cores: 4, MemoryMB: 2048, MaxMemoryMB: 2300}, samples(3, 50, 90), 0, 2300},
		{"メモリが低いと step 分減らす", st(2048, 500), samples(3, 50, 10), 0, 1536},
		{"メモリの減少は下限で切る", st(1200, 100), samples(3, 50, 10), 0, 1024},
		{"使っている量＋余裕より下げない", st(2048, 1600), samples(3, 50, 10), 0, 1856},
		{"余裕がなければ減らさない", st(2048, 1900), samples(3, 50, 10), 0, 0},
		{"両方同時に変える", st(2048, 500), samples(3, 90, 90), 2.5, 2560},
		{"スワップが増えていれば 80% でも増やす", swapping(1280, 1000), samples(3, 50, 80), 0, 1792},
		{"スワップが増えていれば、低くても減らさず増やす", swapping(2048, 500), samples(3, 50, 10), 0, 2560},
		{"スワップが増えていても設定値なら増やさない", State{CPULimit: 2, Cores: 4, MemoryMB: 4096, MaxMemoryMB: 4096, SwapGrowing: true}, samples(3, 50, 80), 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Decide(policy, tt.state, tt.samples)
			if d.NewCPULimit != tt.wantCPU || d.NewMemoryMB != tt.wantMemory {
				t.Errorf("cpu=%v mem=%d, want cpu=%v mem=%d (%s)",
					d.NewCPULimit, d.NewMemoryMB, tt.wantCPU, tt.wantMemory, d.Reason)
			}
			if d.Changed() && d.Reason == "" {
				t.Error("変えるときは理由が必要")
			}
		})
	}
}
