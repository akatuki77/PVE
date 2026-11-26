package proxmox

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// VMConfig は VM の設定のうち、オートスケールに使う部分。
type VMConfig struct {
	Cores    int     // 使える CPU の数（vcpus、なければ cores × sockets）。CPU の上限はこれまで
	MemoryMB int     // メモリの設定値。バルーンで増やせるのはこれまで
	CPULimit float64 // CPU の上限（コア数）。0 なら上限なし
}

// CurrentCPULimit は今使える CPU の量（コア数）を返す。
func (c VMConfig) CurrentCPULimit() float64 {
	return effectiveCPULimit(c.CPULimit, c.Cores)
}

// VMStatus は VM の今の状態のうち、オートスケールに使う部分。
type VMStatus struct {
	Status  string
	Mem     int64 // 使っている量（バイト）
	MaxMem  int64 // 設定値（バイト）
	Balloon int64 // バルーンで今使える量（バイト）。0 ならバルーンなし
	SwapOut int64 // VM の中でスワップに逃がした量の累計（バイト）。バルーンのドライバが報告する
}

// MemLimit は今使えるメモリの量（バイト）を返す。
func (s VMStatus) MemLimit() int64 {
	if s.Balloon > 0 && s.Balloon < s.MaxMem {
		return s.Balloon
	}
	return s.MaxMem
}

func vmPath(node string, vmid int) string {
	return "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid)
}

// GetVMConfig は VM の設定を読む。
func (c *HTTPClient) GetVMConfig(ctx context.Context, node string, vmid int) (VMConfig, error) {
	// 数値が文字列で返ることがあるので any で受ける
	var raw map[string]any
	if err := c.get(ctx, vmPath(node, vmid)+"/config", &raw); err != nil {
		return VMConfig{}, err
	}
	cores := int(toFloat(raw["cores"], 1) * toFloat(raw["sockets"], 1))
	if vcpus := int(toFloat(raw["vcpus"], 0)); vcpus > 0 && vcpus < cores {
		cores = vcpus
	}
	return VMConfig{
		Cores:    cores,
		MemoryMB: int(toFloat(raw["memory"], 512)),
		CPULimit: toFloat(raw["cpulimit"], 0),
	}, nil
}

// GetVMStatus は VM の今の状態を読む。
func (c *HTTPClient) GetVMStatus(ctx context.Context, node string, vmid int) (VMStatus, error) {
	var st VMStatus
	var raw struct {
		Status  string `json:"status"`
		Mem     int64  `json:"mem"`
		MaxMem  int64  `json:"maxmem"`
		Balloon int64  `json:"balloon"`
		Info    struct {
			SwapOut int64 `json:"mem_swapped_out"`
		} `json:"ballooninfo"`
	}
	if err := c.get(ctx, vmPath(node, vmid)+"/status/current", &raw); err != nil {
		return st, err
	}
	return VMStatus{Status: raw.Status, Mem: raw.Mem, MaxMem: raw.MaxMem, Balloon: raw.Balloon, SwapOut: raw.Info.SwapOut}, nil
}

// SetBalloon はバルーンで VM が使えるメモリを mb にする（止めずに変わる）。
func (c *HTTPClient) SetBalloon(ctx context.Context, node string, vmid int, mb int) error {
	var out string
	form := url.Values{"command": {fmt.Sprintf("balloon %d", mb)}}
	if err := c.do(ctx, "POST", vmPath(node, vmid)+"/monitor", form, &out); err != nil {
		return err
	}
	// 成功なら空。何か返ってきたらモニターのエラー
	if out = strings.TrimSpace(out); out != "" {
		return fmt.Errorf("バルーンの変更に失敗: %s", out)
	}
	return nil
}

// SetCPULimit は VM の CPU の上限を cores にする（止めずに変わる）。
func (c *HTTPClient) SetCPULimit(ctx context.Context, node string, vmid int, cores float64) error {
	form := url.Values{"cpulimit": {strconv.FormatFloat(cores, 'f', -1, 64)}}
	return c.do(ctx, "PUT", vmPath(node, vmid)+"/config", form, nil)
}

// toFloat は数値・文字列・"current=2048" の形を数にする。読めなければ def。
func toFloat(v any, def float64) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case string:
		s := x
		if i := strings.Index(s, "current="); i >= 0 {
			s = s[i+len("current="):]
		}
		if i := strings.IndexByte(s, ','); i >= 0 {
			s = s[:i]
		}
		if n, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
			return n
		}
	}
	return def
}
