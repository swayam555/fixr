package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"fixr.in/svc/internal/keys"
	"github.com/redis/go-redis/v9"
	"github.com/twmb/franz-go/pkg/kgo"
)

var (
	ErrOfferGone = errors.New("offer expired or not yours")
)

// accept atomically checks the offer still belongs to this pro and converts it into an assignment.
var acceptScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) ~= ARGV[1] then return 0 end
redis.call('DEL', KEYS[1], KEYS[3], KEYS[4])
redis.call('SET', KEYS[5], ARGV[2], 'EX', 21600)
redis.call('HDEL', KEYS[6], ARGV[2])
return 1`)

var declineScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) ~= ARGV[1] then return 0 end
redis.call('DEL', KEYS[1], KEYS[3], KEYS[4])
redis.call('SADD', KEYS[2], ARGV[1])
redis.call('EXPIRE', KEYS[2], 3600)
return 1`)

func (e *Engine) loadOffer(ctx context.Context, offerID string) (*Offer, error) {
	js, err := e.Redis.Get(ctx, keys.OfferData(offerID)).Result()
	if err != nil {
		return nil, ErrOfferGone
	}
	var of Offer
	if err := json.Unmarshal([]byte(js), &of); err != nil {
		return nil, err
	}
	return &of, nil
}

func (e *Engine) Accept(ctx context.Context, offerID, proID string) (*Offer, error) {
	of, err := e.loadOffer(ctx, offerID)
	if err != nil || of.ProID != proID {
		offersTotal.WithLabelValues("accept_late").Inc()
		return nil, ErrOfferGone
	}
	ok, err := acceptScript.Run(ctx, e.Redis, []string{
		keys.Offer(offerID), keys.Excluded(of.OrderID), keys.ProLock(proID), keys.Pending(of.OrderID),
		keys.ActivePro(proID), keys.OpenOrders(of.Order.ZoneID),
	}, proID, of.OrderID).Int()
	if err != nil {
		return nil, err
	}
	if ok != 1 {
		offersTotal.WithLabelValues("accept_late").Inc()
		return nil, ErrOfferGone
	}
	res, _ := json.Marshal(Result{EventID: newID(), Type: "ACCEPTED", OrderID: of.OrderID, ProID: proID, OfferID: offerID,
		EtaSec: of.EtaSec, AtMs: e.Now().UnixMilli()})
	var produceErr error
	for attempt := 0; attempt < 3; attempt++ {
		if produceErr = e.Kafka.ProduceSync(ctx, &kgo.Record{Topic: TopicResults, Key: []byte(of.OrderID), Value: res}).FirstErr(); produceErr == nil {
			break
		}
		time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
	}
	if produceErr != nil {
		slog.Error("accepted but could not publish result", "order", of.OrderID, "err", produceErr)
		return nil, produceErr
	}
	e.emitOffer(ctx, "ACCEPTED", *of)
	offersTotal.WithLabelValues("accepted").Inc()
	return of, nil
}

func (e *Engine) Decline(ctx context.Context, offerID, proID string) error {
	of, err := e.loadOffer(ctx, offerID)
	if err != nil || of.ProID != proID {
		return ErrOfferGone
	}
	ok, err := declineScript.Run(ctx, e.Redis, []string{
		keys.Offer(offerID), keys.Excluded(of.OrderID), keys.ProLock(proID), keys.Pending(of.OrderID),
	}, proID).Int()
	if err != nil {
		return err
	}
	if ok != 1 {
		return ErrOfferGone
	}
	e.emitOffer(ctx, "DECLINED", *of)
	offersTotal.WithLabelValues("declined").Inc()
	return nil
}

// WatchExpiries turns Redis key-expiry events for offer:* into timeouts: the pro is excluded for this
// order, their lock is freed, and the order goes back into the next round. The pending key expires on
// its own a second later, so a missed notification only costs the exclusion, never the order.
func (e *Engine) WatchExpiries(ctx context.Context) {
	sub := e.Redis.PSubscribe(ctx, "__keyevent@0__:expired")
	defer sub.Close()
	for msg := range sub.Channel() {
		id, ok := strings.CutPrefix(msg.Payload, "offer:")
		if !ok {
			continue
		}
		if first, _ := e.Redis.SetNX(ctx, "offer:expired-handled:"+id, 1, time.Minute).Result(); !first {
			continue
		}
		of, err := e.loadOffer(ctx, id)
		if err != nil {
			continue
		}
		e.Redis.SAdd(ctx, keys.Excluded(of.OrderID), of.ProID)
		e.Redis.Expire(ctx, keys.Excluded(of.OrderID), time.Hour)
		if owner, _ := e.Redis.Get(ctx, keys.ProLock(of.ProID)).Result(); owner == id {
			e.Redis.Del(ctx, keys.ProLock(of.ProID))
		}
		if pending, _ := e.Redis.Get(ctx, keys.Pending(of.OrderID)).Result(); pending == id {
			e.Redis.Del(ctx, keys.Pending(of.OrderID))
		}
		e.emitOffer(ctx, "EXPIRED", *of)
		offersTotal.WithLabelValues("expired").Inc()
	}
}

// OrderEvent is the envelope order-service writes to its outbox.
type OrderEvent struct {
	EventID string          `json:"eventId"`
	Type    string          `json:"type"`
	OrderID string          `json:"orderId"`
	Payload json.RawMessage `json:"payload"`
}

type orderPayload struct {
	ZoneID string `json:"zoneId"`
	ProID  string `json:"proId"`
}

// ApplyOrderEvent keeps the dispatch pool in step with the order state machine. Every write is
// idempotent (HSET/HDEL/SADD), so redelivered events are harmless.
func (e *Engine) ApplyOrderEvent(ctx context.Context, ev OrderEvent) error {
	var p orderPayload
	_ = json.Unmarshal(ev.Payload, &p)
	switch ev.Type {
	case "ORDER_SEARCHING":
		var o OpenOrder
		if err := json.Unmarshal(ev.Payload, &o); err != nil {
			return err
		}
		o.OrderID = ev.OrderID
		js, _ := json.Marshal(o)
		pipe := e.Redis.TxPipeline()
		pipe.HSet(ctx, keys.OpenOrders(o.ZoneID), o.OrderID, js)
		pipe.SAdd(ctx, keys.Zones(), o.ZoneID)
		_, err := pipe.Exec(ctx)
		return err
	case "ORDER_CANCELLED", "ORDER_ASSIGNED", "ORDER_UNFULFILLED":
		if p.ZoneID != "" {
			e.Redis.HDel(ctx, keys.OpenOrders(p.ZoneID), ev.OrderID)
		}
		if ev.Type == "ORDER_CANCELLED" && p.ProID != "" {
			e.releasePro(ctx, p.ProID, ev.OrderID, false)
		}
	case "ORDER_COMPLETED":
		if p.ProID != "" {
			e.releasePro(ctx, p.ProID, ev.OrderID, true)
		}
	case "ORDER_REDISPATCH":
		// pro cancelled after accepting: free them, exclude them, and put the order back in the pool
		if p.ProID != "" {
			e.releasePro(ctx, p.ProID, ev.OrderID, false)
			e.Redis.SAdd(ctx, keys.Excluded(ev.OrderID), p.ProID)
		}
		var o OpenOrder
		if json.Unmarshal(ev.Payload, &o) == nil && o.ZoneID != "" {
			o.OrderID = ev.OrderID
			js, _ := json.Marshal(o)
			e.Redis.HSet(ctx, keys.OpenOrders(o.ZoneID), o.OrderID, js)
			e.Redis.SAdd(ctx, keys.Zones(), o.ZoneID)
		}
	}
	return nil
}

func (e *Engine) releasePro(ctx context.Context, proID, orderID string, completed bool) {
	if cur, _ := e.Redis.Get(ctx, keys.ActivePro(proID)).Result(); cur == orderID {
		e.Redis.Del(ctx, keys.ActivePro(proID))
	}
	if completed {
		e.Redis.HIncrBy(ctx, keys.ProState(proID), "jobs_today", 1)
	}
	e.Redis.HSet(ctx, keys.ProState(proID), "idle_since", e.Now().UnixMilli())
}
