package locproc

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	fixrv1 "fixr.in/svc/gen/fixr/v1"
	"fixr.in/svc/internal/keys"
	"fixr.in/svc/internal/realtime"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redis/go-redis/v9"
	"github.com/uber/h3-go/v4"
	"google.golang.org/protobuf/proto"
)

var (
	processed = promauto.NewCounter(prometheus.CounterOpts{Name: "fixr_locproc_points_total", Help: "Points applied to Redis"})
	fannedOut = promauto.NewCounter(prometheus.CounterOpts{Name: "fixr_locproc_fanout_total", Help: "Points published to pro channels (active jobs only)"})
	lagMs     = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "fixr_locproc_lag_ms", Help: "recorded_at to applied", Buckets: prometheus.ExponentialBuckets(50, 2, 10),
	})
	activePros = promauto.NewGauge(prometheus.GaugeOpts{Name: "fixr_active_pros", Help: "Pros with a fix in the last 60 s"})
)

func Cell(lat, lng float64) string {
	c, err := h3.LatLngToCell(h3.LatLng{Lat: lat, Lng: lng}, keys.H3Res)
	if err != nil {
		return ""
	}
	return c.String()
}

// Frame is what the realtime gateway forwards to a customer tracking this pro.
type Frame struct {
	Type    string  `json:"type"`
	ProID   string  `json:"proId"`
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
	Bearing float32 `json:"bearing"`
	Speed   float32 `json:"speed"`
	Acc     float32 `json:"acc"`
	At      int64   `json:"at"`
}

type Processor struct {
	Redis *redis.Client
	Now   func() time.Time
}

// Apply updates the hot-path indexes for a poll's worth of points in two pipelined round trips.
func (p *Processor) Apply(ctx context.Context, values [][]byte) error {
	latest := map[string]*fixrv1.ProLocation{}
	for _, v := range values {
		loc := &fixrv1.ProLocation{}
		if proto.Unmarshal(v, loc) != nil || loc.GetProId() == "" {
			continue
		}
		if prev, ok := latest[loc.GetProId()]; !ok || loc.GetRecordedAtMs() > prev.GetRecordedAtMs() {
			latest[loc.GetProId()] = loc
		}
	}
	if len(latest) == 0 {
		return nil
	}

	read := p.Redis.Pipeline()
	prevCells := map[string]*redis.StringCmd{}
	active := map[string]*redis.IntCmd{}
	for pro := range latest {
		prevCells[pro] = read.Get(ctx, keys.ProHex(pro))
		active[pro] = read.Exists(ctx, keys.ActivePro(pro))
	}
	if _, err := read.Exec(ctx); err != nil && err != redis.Nil {
		return err
	}

	now := p.Now().UnixMilli()
	write := p.Redis.Pipeline()
	for pro, loc := range latest {
		cell := Cell(loc.GetLat(), loc.GetLng())
		ts := float64(loc.GetRecordedAtMs())
		write.GeoAdd(ctx, keys.ProsGeo, &redis.GeoLocation{Name: pro, Longitude: loc.GetLng(), Latitude: loc.GetLat()})
		write.ZAdd(ctx, keys.ProsSeen, redis.Z{Score: ts, Member: pro})
		write.HSet(ctx, keys.ProState(pro), "lat", loc.GetLat(), "lng", loc.GetLng(), "seen", loc.GetRecordedAtMs(), "mode", loc.GetMode().String())
		if prev := prevCells[pro].Val(); prev != cell {
			if prev != "" {
				write.ZRem(ctx, keys.Hex(prev), pro)
			}
			write.Set(ctx, keys.ProHex(pro), cell, 2*time.Hour)
		}
		write.ZAdd(ctx, keys.Hex(cell), redis.Z{Score: ts, Member: pro})
		if active[pro].Val() == 1 {
			data, _ := json.Marshal(Frame{Type: "pro.location", ProID: pro, Lat: loc.GetLat(), Lng: loc.GetLng(),
				Bearing: loc.GetBearingDeg(), Speed: loc.GetSpeedMps(), Acc: loc.GetAccuracyM(), At: loc.GetRecordedAtMs()})
			env, _ := json.Marshal(realtime.Envelope{Ch: keys.ProChannel(pro), Data: data})
			write.Publish(ctx, keys.Channel(keys.ProChannel(pro)), env)
			fannedOut.Inc()
		}
		lagMs.Observe(float64(now - loc.GetRecordedAtMs()))
	}
	_, err := write.Exec(ctx)
	processed.Add(float64(len(latest)))
	return err
}

// Sweep drops pros with no fix for LivenessMs from the GEO and hex indexes; dispatch never sees ghosts.
func (p *Processor) Sweep(ctx context.Context) (int, error) {
	cutoff := p.Now().UnixMilli() - keys.LivenessMs
	stale, err := p.Redis.ZRangeByScore(ctx, keys.ProsSeen, &redis.ZRangeBy{Min: "-inf", Max: strconv.FormatInt(cutoff, 10)}).Result()
	if err != nil || len(stale) == 0 {
		p.updateGauge(ctx)
		return 0, err
	}
	cells := make([]*redis.StringCmd, len(stale))
	read := p.Redis.Pipeline()
	for i, pro := range stale {
		cells[i] = read.Get(ctx, keys.ProHex(pro))
	}
	_, _ = read.Exec(ctx)
	write := p.Redis.Pipeline()
	for i, pro := range stale {
		write.ZRem(ctx, keys.ProsGeo, pro)
		write.ZRem(ctx, keys.ProsSeen, pro)
		if c := cells[i].Val(); c != "" {
			write.ZRem(ctx, keys.Hex(c), pro)
		}
		write.Del(ctx, keys.ProHex(pro))
	}
	_, err = write.Exec(ctx)
	p.updateGauge(ctx)
	return len(stale), err
}

func (p *Processor) updateGauge(ctx context.Context) {
	if n, err := p.Redis.ZCard(ctx, keys.ProsSeen).Result(); err == nil {
		activePros.Set(float64(n))
	}
}
