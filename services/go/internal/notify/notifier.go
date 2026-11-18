package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"fixr.in/svc/internal/dispatch"
	"fixr.in/svc/internal/keys"
	"fixr.in/svc/internal/realtime"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redis/go-redis/v9"
)

var (
	framesEmitted = promauto.NewCounterVec(prometheus.CounterOpts{Name: "fixr_notifier_frames_total", Help: "Frames emitted"}, []string{"type"})
	pushesSent    = promauto.NewCounterVec(prometheus.CounterOpts{Name: "fixr_notifier_push_total", Help: "Pushes by outcome"}, []string{"outcome"})
	assignLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "fixr_time_to_assign_seconds", Help: "Order searching -> assigned",
		Buckets: []float64{1, 2, 3, 5, 8, 12, 20, 30, 45, 60, 90, 120, 300},
	})
)

type Notifier struct {
	Redis  *redis.Client
	Emit   *realtime.Emitter
	Sender Sender
}

type orderPayload struct {
	CustomerID  string  `json:"customerId"`
	ProID       string  `json:"proId"`
	Status      string  `json:"status"`
	Version     int64   `json:"version"`
	ServiceName string  `json:"serviceName"`
	EtaSec      int     `json:"etaSec"`
	SearchingMs int64   `json:"searchingMs"`
	ProName     string  `json:"proName"`
	Lat         float64 `json:"lat"`
	Lng         float64 `json:"lng"`
}

// OrderFrame is what both apps receive on order:{id}.
type OrderFrame struct {
	Type    string `json:"type"`
	EventID string `json:"eventId"`
	Event   string `json:"event"`
	OrderID string `json:"orderId"`
	Status  string `json:"status"`
	Version int64  `json:"version"`
	ProID   string `json:"proId,omitempty"`
	ProName string `json:"proName,omitempty"`
	EtaSec  int    `json:"etaSec,omitempty"`
	AtMs    int64  `json:"atMs"`
}

var customerCopy = map[string][2]string{
	"ORDER_SEARCHING":   {"Finding your pro", "Matching you with the nearest verified pro."},
	"ORDER_ASSIGNED":    {"Pro assigned", "%s is on the way."},
	"ORDER_EN_ROUTE":    {"On the way", "%s has started the trip."},
	"ORDER_ARRIVED":     {"Your pro is here", "Share the start code when you are ready."},
	"ORDER_IN_PROGRESS": {"Job started", "We will let you know when it is done."},
	"ORDER_COMPLETED":   {"All done", "Rate %s and see your invoice."},
	"ORDER_CANCELLED":   {"Order cancelled", "Any payment will be refunded to the original method."},
	"ORDER_UNFULFILLED": {"No pro available", "Nobody nearby could take it. You have not been charged."},
	"ORDER_REDISPATCH":  {"Finding another pro", "Your pro had to drop out. We are finding the next nearest."},
}

// first returns false when this consumer has already handled the event (Kafka redelivery).
func (n *Notifier) first(ctx context.Context, eventID string) bool {
	if eventID == "" {
		return true
	}
	ok, err := n.Redis.SetNX(ctx, keys.Processed("notifier", eventID), 1, 24*time.Hour).Result()
	return err != nil || ok
}

func (n *Notifier) OnOrderEvent(ctx context.Context, ev dispatch.OrderEvent) error {
	if !n.first(ctx, ev.EventID) {
		return nil
	}
	var p orderPayload
	_ = json.Unmarshal(ev.Payload, &p)
	acl := map[string]any{"customer": p.CustomerID}
	if p.ProID != "" {
		acl["pro"] = p.ProID
	}
	pipe := n.Redis.TxPipeline()
	pipe.HSet(ctx, keys.OrderACL(ev.OrderID), acl)
	pipe.Expire(ctx, keys.OrderACL(ev.OrderID), 48*time.Hour)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}

	now := time.Now().UnixMilli()
	frame := OrderFrame{Type: "order.state", EventID: ev.EventID, Event: ev.Type, OrderID: ev.OrderID, Status: p.Status,
		Version: p.Version, ProID: p.ProID, ProName: p.ProName, EtaSec: p.EtaSec, AtMs: now}
	if _, err := n.Emit.Durable(ctx, keys.OrderChannel(ev.OrderID), frame); err != nil {
		return err
	}
	framesEmitted.WithLabelValues(ev.Type).Inc()
	if ev.Type == "ORDER_ASSIGNED" && p.SearchingMs > 0 {
		assignLatency.Observe(float64(now-p.SearchingMs) / 1000)
	}

	if c, ok := customerCopy[ev.Type]; ok {
		name := p.ProName
		if name == "" {
			name = "Your pro"
		}
		body := c[1]
		if strings.Contains(body, "%s") {
			body = fmt.Sprintf(body, name)
		}
		n.push(ctx, p.CustomerID, Push{Title: c[0], Body: body, Priority: "normal",
			Data: map[string]string{"orderId": ev.OrderID, "event": ev.Type, "url": "fixr://orders/" + ev.OrderID}})
	}
	if ev.Type == "ORDER_CANCELLED" && p.ProID != "" {
		n.push(ctx, p.ProID, Push{Title: "Job cancelled", Body: "The customer cancelled. You are back in the queue.", Priority: "high",
			Data: map[string]string{"orderId": ev.OrderID, "event": ev.Type}})
	}
	return nil
}

// OfferFrame is what the Partner app receives on pro:{id}.
type OfferFrame struct {
	Type    string          `json:"type"`
	EventID string          `json:"eventId"`
	Offer   *dispatch.Offer `json:"offer,omitempty"`
	OfferID string          `json:"offerId"`
	Reason  string          `json:"reason,omitempty"`
	AtMs    int64           `json:"atMs"`
}

func (n *Notifier) OnOfferEvent(ctx context.Context, ev dispatch.OfferEvent) error {
	if !n.first(ctx, ev.EventID) {
		return nil
	}
	ch := keys.ProChannel(ev.Offer.ProID)
	if ev.Type == "OFFERED" {
		of := ev.Offer
		if _, err := n.Emit.Durable(ctx, ch, OfferFrame{Type: "offer", EventID: ev.EventID, Offer: &of, OfferID: of.OfferID, AtMs: ev.AtMs}); err != nil {
			return err
		}
		n.push(ctx, of.ProID, Push{
			Title: "New job · " + of.Order.ServiceName, Body: strconv.Itoa(of.EtaSec/60+1) + " min away · " + of.Order.PriceBand,
			Priority: "high", FullScreen: true,
			Data: map[string]string{"kind": "offer", "offerId": of.OfferID, "orderId": of.OrderID,
				"expiresMs": strconv.FormatInt(of.ExpiresMs, 10), "service": of.Order.ServiceName, "etaSec": strconv.Itoa(of.EtaSec)},
		})
		framesEmitted.WithLabelValues("offer").Inc()
		return nil
	}
	_, err := n.Emit.Durable(ctx, ch, OfferFrame{Type: "offer.closed", EventID: ev.EventID, OfferID: ev.Offer.OfferID, Reason: ev.Type, AtMs: ev.AtMs})
	framesEmitted.WithLabelValues("offer.closed").Inc()
	return err
}

func (n *Notifier) push(ctx context.Context, userID string, p Push) {
	if userID == "" {
		return
	}
	tokens, err := n.Redis.SMembers(ctx, keys.PushTokens(userID)).Result()
	if err != nil || len(tokens) == 0 {
		pushesSent.WithLabelValues("no_token").Inc()
		return
	}
	for _, t := range tokens {
		p.Token = t
		if err := n.Sender.Send(ctx, p); err != nil {
			slog.Warn("push failed", "sender", n.Sender.Name(), "err", err)
			pushesSent.WithLabelValues("error").Inc()
			continue
		}
		pushesSent.WithLabelValues("sent").Inc()
	}
}
