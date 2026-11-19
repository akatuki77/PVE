package web

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBrokerPublish(t *testing.T) {
	tests := []struct {
		name     string
		subs     int // つなぐ画面の数
		unsubs   int // そのうち切る数
		wantRecv int // 届く数
	}{
		{"全員に届く", 3, 0, 3},
		{"切った画面には届かない", 3, 1, 2},
		{"誰もいなくても止まらない", 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBroker()
			var chs []chan Message
			for range tt.subs {
				chs = append(chs, b.subscribe())
			}
			for _, ch := range chs[:tt.unsubs] {
				b.unsubscribe(ch)
			}
			if err := b.Publish("vms", map[string]int{"n": 1}); err != nil {
				t.Fatal(err)
			}
			got := 0
			for _, ch := range chs {
				select {
				case msg := <-ch:
					if msg.Event != "vms" || string(msg.Data) != `{"n":1}` {
						t.Errorf("msg = %s %s", msg.Event, msg.Data)
					}
					got++
				default:
				}
			}
			if got != tt.wantRecv {
				t.Errorf("届いた数 = %d, want %d", got, tt.wantRecv)
			}
		})
	}
}

func TestPublishDoesNotBlock(t *testing.T) {
	b := NewBroker()
	ch := b.subscribe()
	done := make(chan struct{})
	go func() {
		for range 10 {
			b.Publish("vms", 1)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("受け取らない画面があると Publish が止まる")
	}
	if len(ch) != cap(ch) {
		t.Errorf("たまった数 = %d, want %d", len(ch), cap(ch))
	}
}

// readEvent は SSE の1件（空行まで）を読み、event と data を返す。
func readEvent(t *testing.T, r *bufio.Reader) (event, data string) {
	t.Helper()
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("読み取りに失敗: %v", err)
		}
		line = strings.TrimRight(line, "\n")
		switch {
		case line == "":
			return event, data
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		}
	}
}

func TestStream(t *testing.T) {
	b := NewBroker()
	s := NewServer(Options{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:     fakeDB{},
		VMs:    fakeVMs{snap: oneVM},
		Broker: b,
	})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/stream", nil)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q", ct)
	}
	r := bufio.NewReader(resp.Body)

	tests := []struct {
		name      string
		publish   any // nil なら送らない（つないだ直後の1件）
		wantEvent string
		wantData  string
	}{
		{"つないだ直後に最新の一覧が届く", nil, "vms", `"name":"db"`},
		{"Publish したものが届く", map[string]string{"x": "y"}, "vms", `{"x":"y"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.publish != nil {
				b.Publish("vms", tt.publish)
			}
			event, data := readEvent(t, r)
			if event != tt.wantEvent || !strings.Contains(data, tt.wantData) {
				t.Errorf("event = %q, data = %q", event, data)
			}
		})
	}
}
