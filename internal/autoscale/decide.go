// Package autoscale は使用率を見て、VM の CPU の上限とメモリ（バルーン）を増減する。
package autoscale

import (
	"fmt"
	"math"
	"strings"

	"github.com/akatuki77/proxconductor/internal/config"
	"github.com/akatuki77/proxconductor/internal/store"
)

// MinSamples は判定に必要な使用量の件数。
const MinSamples = 3

// State は VM の今の割り当て。
type State struct {
	CPULimit    float64 // 今の CPU の上限（コア数）
	Cores       int     // VM のコア数。CPU の上限はこれまで
	MemoryMB    int     // 今使えるメモリ（バルーンの値）
	MaxMemoryMB int     // メモリの設定値。増やせるのはこれまで
	UsedMB      int     // 今使っているメモリ
	SwapGrowing bool    // VM の中でスワップが増えている（メモリが足りない）
}

// Decision は判定の結果。0 の項目は変えない。
type Decision struct {
	NewCPULimit float64
	NewMemoryMB int
	Reason      string
}

// Changed は変更があるかを返す。
func (d Decision) Changed() bool { return d.NewCPULimit != 0 || d.NewMemoryMB != 0 }

// Decide は使用率の平均としきい値から、変更の内容を決める。
func Decide(p config.Policy, st State, samples []store.Metric) Decision {
	if len(samples) < MinSamples || st.CPULimit <= 0 || st.MemoryMB <= 0 {
		return Decision{}
	}
	var cpuSum, memSum float64
	for _, s := range samples {
		// s.CPU は全コアに対する割合なので、使ったコア数に直して上限で割る
		cpuSum += s.CPU * float64(s.CPUs) / st.CPULimit * 100
		if s.MemMax > 0 {
			memSum += float64(s.MemUsed) / float64(s.MemMax) * 100
		}
	}
	n := float64(len(samples))
	cpuAvg, memAvg := cpuSum/n, memSum/n

	var d Decision
	var reasons []string

	cpuMax := math.Min(p.CPU.Max, float64(st.Cores))
	switch {
	case cpuAvg > p.CPU.UpThreshold && st.CPULimit < cpuMax:
		d.NewCPULimit = round2(math.Min(st.CPULimit+p.CPU.Step, cpuMax))
		reasons = append(reasons, fmt.Sprintf("CPU %.0f%% > %.0f%% のため上限 %g → %g コア", cpuAvg, p.CPU.UpThreshold, st.CPULimit, d.NewCPULimit))
	case cpuAvg < p.CPU.DownThreshold && st.CPULimit > p.CPU.Min:
		d.NewCPULimit = round2(math.Max(st.CPULimit-p.CPU.Step, p.CPU.Min))
		reasons = append(reasons, fmt.Sprintf("CPU %.0f%% < %.0f%% のため上限 %g → %g コア", cpuAvg, p.CPU.DownThreshold, st.CPULimit, d.NewCPULimit))
	}

	memMax := min(p.Memory.MaxMB, st.MaxMemoryMB)
	switch {
	case (memAvg > p.Memory.UpThreshold || st.SwapGrowing) && st.MemoryMB < memMax:
		d.NewMemoryMB = min(st.MemoryMB+p.Memory.StepMB, memMax)
		why := fmt.Sprintf("メモリ %.0f%% > %.0f%%", memAvg, p.Memory.UpThreshold)
		if memAvg <= p.Memory.UpThreshold {
			why = fmt.Sprintf("VM の中でスワップが増えた（メモリ %.0f%%）", memAvg)
		}
		reasons = append(reasons, fmt.Sprintf("%s のため %d → %d MB", why, st.MemoryMB, d.NewMemoryMB))
	case memAvg < p.Memory.DownThreshold && st.MemoryMB > p.Memory.MinMB && !st.SwapGrowing:
		// 使っている量＋余裕より下には減らさない
		floor := max(p.Memory.MinMB, st.UsedMB+p.Memory.MarginMB)
		if next := max(st.MemoryMB-p.Memory.StepMB, floor); next < st.MemoryMB {
			d.NewMemoryMB = next
			reasons = append(reasons, fmt.Sprintf("メモリ %.0f%% < %.0f%% のため %d → %d MB", memAvg, p.Memory.DownThreshold, st.MemoryMB, d.NewMemoryMB))
		}
	}

	d.Reason = strings.Join(reasons, "、")
	return d
}

// round2 は小数第2位までに丸める。
func round2(x float64) float64 { return math.Round(x*100) / 100 }
