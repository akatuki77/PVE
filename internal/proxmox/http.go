package proxmox

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HTTPClient は API トークンで Proxmox API を呼ぶ。
type HTTPClient struct {
	baseURL string
	auth    string // ログに出さない
	http    *http.Client
}

// NewHTTPClient は HTTPClient を作る。insecure は自己署名証明書を許可する。
func NewHTTPClient(apiURL, tokenID, tokenSecret string, insecure bool) *HTTPClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	base := strings.TrimSuffix(strings.TrimRight(apiURL, "/"), "/api2/json")
	return &HTTPClient{
		baseURL: base + "/api2/json",
		auth:    "PVEAPIToken=" + tokenID + "=" + tokenSecret,
		http:    &http.Client{Transport: transport, Timeout: 15 * time.Second},
	}
}

func (c *HTTPClient) get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, out)
}

// do は API を呼び、応答の data を out に読む。form が nil でなければ本文として送る。
func (c *HTTPClient) do(ctx context.Context, method, path string, form url.Values, out any) error {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("リクエストの作成に失敗: %w", err)
	}
	req.Header.Set("Authorization", c.auth)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s に失敗: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s %s が失敗: %s %s", method, path, resp.Status, strings.TrimSpace(string(msg)))
	}
	if out == nil {
		return nil
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("%s %s の応答の解析に失敗: %w", method, path, err)
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("%s %s のデータの解析に失敗: %w", method, path, err)
	}
	return nil
}

// ListVMs はクラスタ内の VM を返す（LXC とテンプレートは除く）。
func (c *HTTPClient) ListVMs(ctx context.Context) ([]VM, error) {
	var raw []struct {
		Type     string  `json:"type"`
		VMID     int     `json:"vmid"`
		Name     string  `json:"name"`
		Node     string  `json:"node"`
		Status   string  `json:"status"`
		CPU      float64 `json:"cpu"`
		MaxCPU   int     `json:"maxcpu"`
		Mem      int64   `json:"mem"`
		MaxMem   int64   `json:"maxmem"`
		Tags     string  `json:"tags"`
		Template int     `json:"template"`
	}
	if err := c.get(ctx, "/cluster/resources?type=vm", &raw); err != nil {
		return nil, err
	}
	vms := make([]VM, 0, len(raw))
	for _, r := range raw {
		if r.Type != "qemu" || r.Template == 1 {
			continue
		}
		vms = append(vms, VM{
			VMID: r.VMID, Name: r.Name, Node: r.Node, Status: r.Status,
			CPU: r.CPU, MaxCPU: r.MaxCPU, Mem: r.Mem, MaxMem: r.MaxMem,
			Tags: splitTags(r.Tags),
		})
	}
	return vms, nil
}

// splitTags は "a;b" 形式のタグを分ける。
func splitTags(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == ',' || r == ' ' })
}
