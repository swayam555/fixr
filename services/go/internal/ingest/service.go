package ingest

import (
	"context"
	"fmt"
	"strconv"
	"time"

	fixrv1 "fixr.in/svc/gen/fixr/v1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redis/go-redis/v9"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"
)

const Topic = "pro.locations"

var (
	pointsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fixr_location_points_total", Help: "Location fixes received, by outcome",
	}, []string{"outcome"})
	batchesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fixr_location_batches_total", Help: "Location batches, by outcome",
	}, []string{"outcome"})
	batchSize = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "fixr_location_batch_size", Help: "Fixes per batch", Buckets: []float64{1, 2, 5, 10, 20, 50},
	})
)

type Publisher interface {
	ProduceSync(ctx context.Context, rs ...*kgo.Record) kgo.ProduceResults
}

type Service struct {
	Redis *redis.Client
	Kafka Publisher
	Rules Rules
	Now   func() time.Time
}

func lastKey(pro string) string { return "pro:last:" + pro }

func (s *Service) Ingest(ctx context.Context, proID string, b *fixrv1.LocationBatch) (*fixrv1.BatchAck, error) {
	ack := &fixrv1.BatchAck{BatchId: b.GetBatchId()}
	if b.GetBatchId() != "" {
		fresh, err := s.Redis.SetNX(ctx, "locbatch:"+proID+":"+b.GetBatchId(), 1, 10*time.Minute).Result()
		if err != nil {
			return nil, err
		}
		if !fresh {
			batchesTotal.WithLabelValues("duplicate").Inc()
			ack.Duplicate = true
			return ack, nil
		}
	}
	batchSize.Observe(float64(len(b.GetPoints())))

	last, err := s.loadLast(ctx, proID)
	if err != nil {
		return nil, err
	}
	fixes := make([]Fix, len(b.GetPoints()))
	byTime := make(map[int64]*fixrv1.LocationPoint, len(fixes))
	for i, p := range b.GetPoints() {
		fixes[i] = Fix{Lat: p.GetLat(), Lng: p.GetLng(), AccuracyM: float64(p.GetAccuracyM()), RecordedAt: p.GetRecordedAtMs()}
		byTime[p.GetRecordedAtMs()] = p
	}
	now := s.Now()
	verdicts, newLast := Filter(fixes, last, now.UnixMilli(), s.Rules)

	records := make([]*kgo.Record, 0, len(verdicts))
	for _, v := range verdicts {
		if v.Reason != Accepted {
			ack.Rejected++
			pointsTotal.WithLabelValues(string(v.Reason)).Inc()
			continue
		}
		src := byTime[v.Fix.RecordedAt]
		value, err := proto.Marshal(&fixrv1.ProLocation{
			ProId: proID, Lat: v.Fix.Lat, Lng: v.Fix.Lng, AccuracyM: src.GetAccuracyM(),
			SpeedMps: src.GetSpeedMps(), BearingDeg: src.GetBearingDeg(), RecordedAtMs: v.Fix.RecordedAt,
			ReceivedAtMs: now.UnixMilli(), Activity: src.GetActivity(), Mode: b.GetMode(),
		})
		if err != nil {
			return nil, err
		}
		records = append(records, &kgo.Record{Topic: Topic, Key: []byte(proID), Value: value})
	}
	if len(records) > 0 {
		if err := s.Kafka.ProduceSync(ctx, records...).FirstErr(); err != nil {
			// let the client retry the whole batch: release the dedupe key
			s.Redis.Del(ctx, "locbatch:"+proID+":"+b.GetBatchId())
			batchesTotal.WithLabelValues("kafka_error").Inc()
			return nil, fmt.Errorf("publish: %w", err)
		}
		ack.Accepted = uint32(len(records))
		pointsTotal.WithLabelValues("accepted").Add(float64(len(records)))
	}
	if newLast != nil && newLast != last {
		s.Redis.HSet(ctx, lastKey(proID), "lat", newLast.Lat, "lng", newLast.Lng, "acc", newLast.AccuracyM, "ts", newLast.RecordedAt)
		s.Redis.Expire(ctx, lastKey(proID), 2*time.Hour)
	}
	batchesTotal.WithLabelValues("ok").Inc()
	return ack, nil
}

func (s *Service) loadLast(ctx context.Context, proID string) (*Fix, error) {
	m, err := s.Redis.HGetAll(ctx, lastKey(proID)).Result()
	if err != nil || len(m) == 0 {
		return nil, err
	}
	f := &Fix{}
	f.Lat, _ = strconv.ParseFloat(m["lat"], 64)
	f.Lng, _ = strconv.ParseFloat(m["lng"], 64)
	f.AccuracyM, _ = strconv.ParseFloat(m["acc"], 64)
	f.RecordedAt, _ = strconv.ParseInt(m["ts"], 10, 64)
	return f, nil
}
