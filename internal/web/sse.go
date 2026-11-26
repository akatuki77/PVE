package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Message は画面に送る通知1件。
type Message struct {
	Event string
	Data  []byte
}

// Broker は接続中の画面に通知を配る。
type Broker struct {
	mu   sync.Mutex // subs 用
	subs map[chan Message]struct{}
}

// NewBroker は Broker を作る。
func NewBroker() *Broker {
	return &Broker{subs: map[chan Message]struct{}{}}
}

func (b *Broker) subscribe() chan Message {
	ch := make(chan Message, 4)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *Broker) unsubscribe(ch chan Message) {
	b.mu.Lock()
	delete(b.subs, ch)
	b.mu.Unlock()
}

// Publish は v を JSON にして全員に送る。受け取りが遅い画面には送らない。
func (b *Broker) Publish(event string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("通知のJSON化に失敗: %w", err)
	}
	msg := Message{Event: event, Data: data}
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- msg:
		default:
		}
	}
	return nil
}

const keepAliveInterval = 25 * time.Second

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	ch := s.broker.subscribe()
	defer s.broker.unsubscribe(ch)

	rc := http.NewResponseController(w)
	res, err := s.vmsResponse(r.Context(), s.vms.Latest())
	if err != nil {
		s.logger.Error("VM一覧の作成に失敗", "err", err)
		return
	}
	first, err := json.Marshal(res)
	if err != nil {
		s.logger.Error("通知のJSON化に失敗", "err", err)
		return
	}
	writeEvent(w, Message{Event: "vms", Data: first})
	if err := rc.Flush(); err != nil {
		return
	}

	keepAlive := time.NewTicker(keepAliveInterval)
	defer keepAlive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			writeEvent(w, msg)
		case <-keepAlive.C:
			fmt.Fprint(w, ": keep-alive\n\n")
		}
		if err := rc.Flush(); err != nil {
			return
		}
	}
}

// writeEvent は SSE の形式で1件書く。
func writeEvent(w http.ResponseWriter, msg Message) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", msg.Event, msg.Data)
}
