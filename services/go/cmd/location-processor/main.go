package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"fixr.in/svc/internal/ingest"
	"fixr.in/svc/internal/locproc"
	"fixr.in/svc/internal/platform"
	"github.com/twmb/franz-go/pkg/kgo"
)

func main() {
	platform.NewLogger("location-processor")
	ctx, cancel := platform.SignalContext()
	defer cancel()

	consumer, err := platform.NewConsumer("location-processor", []string{ingest.Topic})
	if err != nil {
		slog.Error("kafka", "err", err)
		os.Exit(1)
	}
	defer consumer.Close()
	proc := &locproc.Processor{Redis: platform.NewRedis(), Now: time.Now}

	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if n, err := proc.Sweep(ctx); err != nil {
					slog.Warn("sweep", "err", err)
				} else if n > 0 {
					slog.Info("swept stale pros", "count", n)
				}
			}
		}
	}()

	go func() {
		_ = platform.Run(ctx, platform.Env("ADDR", ":8094"), http.NewServeMux())
	}()

	for ctx.Err() == nil {
		fetches := consumer.PollRecords(ctx, 2000)
		fetches.EachError(func(t string, p int32, err error) { slog.Warn("fetch", "topic", t, "partition", p, "err", err) })
		values := make([][]byte, 0, fetches.NumRecords())
		fetches.EachRecord(func(r *kgo.Record) { values = append(values, r.Value) })
		if len(values) > 0 {
			if err := proc.Apply(ctx, values); err != nil {
				slog.Error("apply", "err", err)
				consumer.AllowRebalance()
				time.Sleep(time.Second)
				continue
			}
			if err := consumer.CommitUncommittedOffsets(context.Background()); err != nil {
				slog.Warn("commit", "err", err)
			}
		}
		consumer.AllowRebalance()
	}
}
