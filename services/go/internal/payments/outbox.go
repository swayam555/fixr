package payments

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/twmb/franz-go/pkg/kgo"
)

// PublishOutbox drains pay.outbox to payments.events in order. The transaction-scoped advisory lock
// means one replica publishes at a time; a row is stamped only after Kafka acks it.
func (s *Service) PublishOutbox(ctx context.Context, kc *kgo.Client, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if err := s.publishBatch(ctx, kc); err != nil && ctx.Err() == nil {
			slog.Warn("payments outbox", "err", err)
		}
	}
}

func (s *Service) publishBatch(ctx context.Context, kc *kgo.Client) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		var got bool
		if err := tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(hashtext('fixr.payments.outbox'))").Scan(&got); err != nil || !got {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id, msg_key, payload::text FROM pay.outbox WHERE published_at IS NULL ORDER BY created_at LIMIT 200`)
		if err != nil {
			return err
		}
		type row struct{ id, key, payload string }
		batch, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (row, error) {
			var x row
			return x, r.Scan(&x.id, &x.key, &x.payload)
		})
		if err != nil || len(batch) == 0 {
			return err
		}
		for _, x := range batch {
			if err := kc.ProduceSync(ctx, &kgo.Record{Topic: TopicPayments, Key: []byte(x.key), Value: []byte(x.payload)}).FirstErr(); err != nil {
				// keep the stamps for rows already acked; the rest go out on the next tick, still in order
				slog.Warn("payments outbox publish", "err", err)
				return nil
			}
			if _, err := tx.Exec(ctx, `UPDATE pay.outbox SET published_at = now() WHERE id = $1`, x.id); err != nil {
				return err
			}
		}
		return nil
	})
}
