package realtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"

	"fixr.in/svc/internal/keys"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redis/go-redis/v9"
)

var (
	connections = promauto.NewGauge(prometheus.GaugeOpts{Name: "fixr_ws_connections", Help: "Open WebSocket connections on this node"})
	redisSubs   = promauto.NewGauge(prometheus.GaugeOpts{Name: "fixr_ws_redis_channels", Help: "Redis channels this node is subscribed to"})
	framesOut   = promauto.NewCounterVec(prometheus.CounterOpts{Name: "fixr_ws_frames_total", Help: "Frames sent to clients"}, []string{"kind"})
	dropped     = promauto.NewCounterVec(prometheus.CounterOpts{Name: "fixr_ws_dropped_total", Help: "Frames dropped or coalesced"}, []string{"reason"})
)

// Hub owns this node's single Redis pub/sub connection. It subscribes to a Redis channel only while
// at least one local client wants it, so a node's Redis traffic scales with its clients, not the city.
type Hub struct {
	redis *redis.Client
	ps    *redis.PubSub

	mu   sync.RWMutex
	subs map[string]map[*Conn]struct{} // logical channel -> local conns
}

func NewHub(ctx context.Context, rdb *redis.Client) *Hub {
	h := &Hub{redis: rdb, ps: rdb.Subscribe(ctx), subs: map[string]map[*Conn]struct{}{}}
	go h.pump(ctx)
	return h
}

func (h *Hub) pump(ctx context.Context) {
	for msg := range h.ps.Channel(redis.WithChannelSize(4096)) {
		var env Envelope
		if json.Unmarshal([]byte(msg.Payload), &env) != nil {
			continue
		}
		ch := strings.TrimPrefix(msg.Channel, "ch:")
		h.mu.RLock()
		for c := range h.subs[ch] {
			c.deliver(env)
		}
		h.mu.RUnlock()
	}
	_ = ctx
}

func (h *Hub) add(ctx context.Context, ch string, c *Conn) error {
	h.mu.Lock()
	set, ok := h.subs[ch]
	if !ok {
		set = map[*Conn]struct{}{}
		h.subs[ch] = set
	}
	set[c] = struct{}{}
	first := !ok
	h.mu.Unlock()
	if first {
		redisSubs.Inc()
		return h.ps.Subscribe(ctx, keys.Channel(ch))
	}
	return nil
}

func (h *Hub) remove(ctx context.Context, ch string, c *Conn) {
	h.mu.Lock()
	set := h.subs[ch]
	delete(set, c)
	last := set != nil && len(set) == 0
	if last {
		delete(h.subs, ch)
	}
	h.mu.Unlock()
	if last {
		redisSubs.Dec()
		_ = h.ps.Unsubscribe(ctx, keys.Channel(ch))
	}
}

// History returns durable frames after seq (exclusive), oldest first.
func (h *Hub) History(ctx context.Context, ch, after string) ([]Envelope, error) {
	start := "-"
	if after != "" {
		start = "(" + after
	}
	msgs, err := h.redis.XRange(ctx, keys.Channel(ch), start, "+").Result()
	if err != nil {
		return nil, err
	}
	out := make([]Envelope, 0, len(msgs))
	for _, m := range msgs {
		d, _ := m.Values["d"].(string)
		out = append(out, Envelope{Ch: ch, Seq: m.ID, Data: json.RawMessage(d)})
	}
	return out, nil
}

func (h *Hub) Close() {
	if err := h.ps.Close(); err != nil {
		slog.Warn("pubsub close", "err", err)
	}
}
