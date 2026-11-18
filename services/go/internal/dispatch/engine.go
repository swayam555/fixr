package dispatch

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"strconv"
	"time"

	"fixr.in/svc/internal/geo"
	"fixr.in/svc/internal/keys"
	"fixr.in/svc/internal/match"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redis/go-redis/v9"
	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	TopicOffers  = "dispatch.offers"
	TopicResults = "dispatch.results"
)

var (
	roundsTotal  = promauto.NewCounterVec(prometheus.CounterOpts{Name: "fixr_dispatch_rounds_total", Help: "Matching rounds"}, []string{"zone"})
	offersTotal  = promauto.NewCounterVec(prometheus.CounterOpts{Name: "fixr_dispatch_offers_total", Help: "Offers by outcome"}, []string{"outcome"})
	roundSeconds = promauto.NewHistogram(prometheus.HistogramOpts{Name: "fixr_dispatch_round_seconds", Help: "Round duration", Buckets: prometheus.ExponentialBuckets(0.002, 2, 10)})
	openOrders   = promauto.NewGaugeVec(prometheus.GaugeOpts{Name: "fixr_dispatch_open_orders", Help: "Orders waiting for a pro"}, []string{"zone"})
	etaSource    = promauto.NewCounterVec(prometheus.CounterOpts{Name: "fixr_dispatch_eta_source_total", Help: "Which ETA model scored a round"}, []string{"source"})
)

type Config struct {
	Round         time.Duration
	OfferTTL      time.Duration
	RadiiM        []float64     // expanding search rings
	RingAfter     time.Duration // order age before moving to the next ring
	GiveUpAfter   time.Duration
	MaxCandidates int
	Strategy      string // "batch" (Hungarian) or "greedy"
	Weights       match.Weights
}

var DefaultConfig = Config{
	Round: 2 * time.Second, OfferTTL: 15 * time.Second, RadiiM: []float64{1000, 3000, 5000},
	RingAfter: 20 * time.Second, GiveUpAfter: 5 * time.Minute, MaxCandidates: 25, Strategy: "batch",
	Weights: match.DefaultWeights,
}

type Engine struct {
	Redis    *redis.Client
	Kafka    *kgo.Client
	Eta      EtaModel
	Cfg      Config
	Now      func() time.Time
	Instance string
}

func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Run ticks every Cfg.Round and matches each zone it can lead. Leadership is a short Redis lease,
// so two dispatch instances split zones and a crashed instance's zones move within a few seconds.
func (e *Engine) Run(ctx context.Context) {
	t := time.NewTicker(e.Cfg.Round)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			zones, err := e.Redis.SMembers(ctx, keys.Zones()).Result()
			if err != nil {
				slog.Warn("zones", "err", err)
				continue
			}
			for _, z := range zones {
				ok, _ := e.Redis.SetNX(ctx, keys.ZoneLeader(z), e.Instance, e.Cfg.Round+time.Second).Result()
				if !ok {
					owner, _ := e.Redis.Get(ctx, keys.ZoneLeader(z)).Result()
					if owner != e.Instance {
						continue
					}
					e.Redis.PExpire(ctx, keys.ZoneLeader(z), e.Cfg.Round+time.Second)
				}
				if err := e.Round(ctx, z); err != nil {
					slog.Warn("round", "zone", z, "err", err)
				}
			}
		}
	}
}

type candidate struct {
	pro  match.Pro
	dist map[int]float64 // order index -> metres
}

// Round runs one micro-batch for a zone: collect open orders without a live offer, gather
// candidates from expanding rings, score every pair, solve the assignment, and send offers.
func (e *Engine) Round(ctx context.Context, zone string) error {
	start := time.Now()
	defer func() { roundSeconds.Observe(time.Since(start).Seconds()) }()
	roundsTotal.WithLabelValues(zone).Inc()
	now := e.Now()

	raw, err := e.Redis.HGetAll(ctx, keys.OpenOrders(zone)).Result()
	if err != nil {
		return err
	}
	openOrders.WithLabelValues(zone).Set(float64(len(raw)))
	if len(raw) == 0 {
		e.Redis.SRem(ctx, keys.Zones(), zone)
		return nil
	}

	var orders []OpenOrder
	for id, js := range raw {
		var o OpenOrder
		if json.Unmarshal([]byte(js), &o) != nil {
			e.Redis.HDel(ctx, keys.OpenOrders(zone), id)
			continue
		}
		age := now.Sub(time.UnixMilli(o.CreatedMs))
		if age > e.Cfg.GiveUpAfter {
			e.giveUp(ctx, zone, o)
			continue
		}
		if n, _ := e.Redis.Exists(ctx, keys.Pending(o.OrderID)).Result(); n == 1 {
			continue
		}
		orders = append(orders, o)
	}
	if len(orders) == 0 {
		return nil
	}

	cands, excluded, err := e.candidates(ctx, orders, now)
	if err != nil {
		return err
	}
	if len(cands) == 0 {
		return nil
	}

	var pairs []EtaPair
	type ref struct{ o, p int }
	var refs []ref
	for pi, c := range cands {
		for oi, d := range c.dist {
			o := orders[oi]
			pairs = append(pairs, EtaPair{ProLat: c.pro.Loc.Lat, ProLng: c.pro.Loc.Lng, OrderLat: o.Lat, OrderLng: o.Lng,
				DistanceM: d, HourOfDay: now.Hour(), Skill: o.Skill, ProJobsToday: c.pro.JobsToday})
			refs = append(refs, ref{oi, pi})
		}
	}
	eta, source := WithFallback(ctx, e.Eta, pairs)
	etaSource.WithLabelValues(source).Inc()

	cost := make([][]float64, len(orders))
	etaOf := make([][]float64, len(orders))
	for i := range cost {
		cost[i] = make([]float64, len(cands))
		etaOf[i] = make([]float64, len(cands))
		for j := range cost[i] {
			cost[i][j] = match.Infeasible
		}
	}
	for k, r := range refs {
		o := orders[r.o]
		mo := match.Order{ID: o.OrderID, Loc: geo.Point{Lat: o.Lat, Lng: o.Lng}, Skill: o.Skill, SkillLevel: o.SkillLevel,
			CreatedMs: o.CreatedMs, Excluded: excluded[o.OrderID]}
		cost[r.o][r.p] = match.Cost(mo, cands[r.p].pro, eta[k], now.UnixMilli(), e.Cfg.Weights)
		etaOf[r.o][r.p] = eta[k]
	}

	var assign []int
	if e.Cfg.Strategy == "greedy" {
		assign = match.Greedy(cost)
	} else {
		assign = match.Hungarian(cost)
	}

	round := now.UnixMilli()
	for oi, pi := range assign {
		if pi < 0 {
			continue
		}
		e.offer(ctx, orders[oi], cands[pi], int(etaOf[oi][pi]), int(cands[pi].dist[oi]), source, round)
	}
	return nil
}

func (e *Engine) candidates(ctx context.Context, orders []OpenOrder, now time.Time) ([]candidate, map[string]map[string]bool, error) {
	byPro := map[string]*candidate{}
	excluded := map[string]map[string]bool{}
	for oi, o := range orders {
		ring := int(now.Sub(time.UnixMilli(o.CreatedMs)) / e.Cfg.RingAfter)
		if ring >= len(e.Cfg.RadiiM) {
			ring = len(e.Cfg.RadiiM) - 1
		}
		hits, err := e.Redis.GeoSearchLocation(ctx, keys.ProsGeo, &redis.GeoSearchLocationQuery{
			GeoSearchQuery: redis.GeoSearchQuery{Longitude: o.Lng, Latitude: o.Lat, Radius: e.Cfg.RadiiM[ring], RadiusUnit: "m",
				Sort: "ASC", Count: e.Cfg.MaxCandidates},
			WithCoord: true, WithDist: true,
		}).Result()
		if err != nil {
			return nil, nil, err
		}
		ex, _ := e.Redis.SMembers(ctx, keys.Excluded(o.OrderID)).Result()
		excluded[o.OrderID] = map[string]bool{}
		for _, p := range ex {
			excluded[o.OrderID][p] = true
		}
		for _, h := range hits {
			c, ok := byPro[h.Name]
			if !ok {
				c = &candidate{pro: match.Pro{ID: h.Name, Loc: geo.Point{Lat: h.Latitude, Lng: h.Longitude}}, dist: map[int]float64{}}
				byPro[h.Name] = c
			}
			c.dist[oi] = h.Dist
		}
	}
	if len(byPro) == 0 {
		return nil, excluded, nil
	}

	pipe := e.Redis.Pipeline()
	state := map[string]*redis.MapStringStringCmd{}
	locked := map[string]*redis.IntCmd{}
	active := map[string]*redis.IntCmd{}
	for id := range byPro {
		state[id] = pipe.HGetAll(ctx, keys.ProState(id))
		locked[id] = pipe.Exists(ctx, keys.ProLock(id))
		active[id] = pipe.Exists(ctx, keys.ActivePro(id))
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, nil, err
	}

	var out []candidate
	for id, c := range byPro {
		s := state[id].Val()
		if s["shift"] != "1" || locked[id].Val() == 1 || active[id].Val() == 1 {
			continue
		}
		c.pro.Skills = parseSkills(s["skills"])
		c.pro.Rating, _ = strconv.ParseFloat(s["rating"], 64)
		if c.pro.Rating == 0 {
			c.pro.Rating = 4.5
		}
		c.pro.JobsToday, _ = strconv.Atoi(s["jobs_today"])
		c.pro.IdleSinceMs, _ = strconv.ParseInt(s["idle_since"], 10, 64)
		if c.pro.IdleSinceMs == 0 {
			c.pro.IdleSinceMs = now.UnixMilli()
		}
		out = append(out, *c)
	}
	return out, excluded, nil
}

// parseSkills reads "plumbing:2,cleaning:1".
func parseSkills(s string) map[string]int {
	out := map[string]int{}
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			part := s[start:i]
			for j := 0; j < len(part); j++ {
				if part[j] == ':' {
					lvl, _ := strconv.Atoi(part[j+1:])
					out[part[:j]] = lvl
				}
			}
			start = i + 1
		}
	}
	return out
}

func (e *Engine) offer(ctx context.Context, o OpenOrder, c candidate, etaSec, distM int, source string, round int64) {
	offerID := newID()
	// the per-pro lock is the guarantee against double assignment, across rounds and instances
	got, err := e.Redis.SetNX(ctx, keys.ProLock(c.pro.ID), offerID, e.Cfg.OfferTTL+time.Second).Result()
	if err != nil || !got {
		offersTotal.WithLabelValues("lock_lost").Inc()
		return
	}
	now := e.Now()
	of := Offer{OfferID: offerID, OrderID: o.OrderID, ProID: c.pro.ID, EtaSec: etaSec, EtaSource: source, DistanceM: distM,
		ExpiresMs: now.Add(e.Cfg.OfferTTL).UnixMilli(), Round: round, Order: o}
	js, _ := json.Marshal(of)
	pipe := e.Redis.TxPipeline()
	pipe.Set(ctx, keys.Offer(offerID), c.pro.ID, e.Cfg.OfferTTL)
	pipe.Set(ctx, keys.OfferData(offerID), js, 10*time.Minute)
	pipe.Set(ctx, keys.Pending(o.OrderID), offerID, e.Cfg.OfferTTL+time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		e.Redis.Del(ctx, keys.ProLock(c.pro.ID))
		return
	}
	e.emitOffer(ctx, "OFFERED", of)
	offersTotal.WithLabelValues("sent").Inc()
}

func (e *Engine) emitOffer(ctx context.Context, typ string, of Offer) {
	ev, _ := json.Marshal(OfferEvent{EventID: newID(), Type: typ, Offer: of, AtMs: e.Now().UnixMilli()})
	e.Kafka.Produce(ctx, &kgo.Record{Topic: TopicOffers, Key: []byte(of.ProID), Value: ev}, func(_ *kgo.Record, err error) {
		if err != nil {
			slog.Error("produce offer", "err", err)
		}
	})
}

func (e *Engine) giveUp(ctx context.Context, zone string, o OpenOrder) {
	e.Redis.HDel(ctx, keys.OpenOrders(zone), o.OrderID)
	ev, _ := json.Marshal(Result{EventID: newID(), Type: "UNFULFILLED", OrderID: o.OrderID, AtMs: e.Now().UnixMilli()})
	e.Kafka.Produce(ctx, &kgo.Record{Topic: TopicResults, Key: []byte(o.OrderID), Value: ev}, nil)
	offersTotal.WithLabelValues("unfulfilled").Inc()
}
