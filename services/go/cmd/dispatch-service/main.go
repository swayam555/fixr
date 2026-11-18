package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

	"fixr.in/svc/internal/dispatch"
	"fixr.in/svc/internal/platform"
	"github.com/twmb/franz-go/pkg/kgo"
)

func main() {
	platform.NewLogger("dispatch-service")
	ctx, cancel := platform.SignalContext()
	defer cancel()

	verifier, err := platform.NewVerifier(ctx, platform.Env("JWKS_URL", "http://localhost:8080/.well-known/jwks.json"), platform.Env("JWT_ISSUER", "https://fixr.local"))
	if err != nil {
		slog.Error("jwks", "err", err)
		os.Exit(1)
	}
	producer, err := platform.NewProducer()
	if err != nil {
		slog.Error("kafka", "err", err)
		os.Exit(1)
	}
	defer producer.Close()

	cfg := dispatch.DefaultConfig
	cfg.Strategy = platform.Env("DISPATCH_STRATEGY", "batch")
	cfg.Round = platform.EnvDuration("DISPATCH_ROUND", cfg.Round)
	host, _ := os.Hostname()
	engine := &dispatch.Engine{
		Redis: platform.NewRedis(), Kafka: producer, Cfg: cfg, Now: time.Now, Instance: host,
		Eta: &dispatch.HTTPEta{URL: platform.Env("ETA_URL", "http://localhost:8000/ai/eta"), Client: &http.Client{}},
	}

	go engine.Run(ctx)
	go engine.WatchExpiries(ctx)
	go consumeOrders(ctx, engine)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /dispatch/offers/{id}/accept", verifier.RequireRole("PRO", engine.AcceptHandler))
	mux.HandleFunc("POST /dispatch/offers/{id}/decline", verifier.RequireRole("PRO", engine.DeclineHandler))
	if err := platform.Run(ctx, platform.Env("ADDR", ":8092"), mux); err != nil {
		slog.Error("server", "err", err)
	}
}

func consumeOrders(ctx context.Context, engine *dispatch.Engine) {
	consumer, err := platform.NewConsumer("dispatch-service", []string{"order.events"})
	if err != nil {
		slog.Error("kafka consumer", "err", err)
		os.Exit(1)
	}
	defer consumer.Close()
	for ctx.Err() == nil {
		fetches := consumer.PollRecords(ctx, 500)
		failed := false
		fetches.EachRecord(func(r *kgo.Record) {
			var ev dispatch.OrderEvent
			if json.Unmarshal(r.Value, &ev) != nil {
				return
			}
			if err := engine.ApplyOrderEvent(ctx, ev); err != nil {
				slog.Error("order event", "type", ev.Type, "order", ev.OrderID, "err", err)
				failed = true
			}
		})
		if !failed {
			_ = consumer.CommitUncommittedOffsets(context.Background())
		} else {
			time.Sleep(time.Second)
		}
		consumer.AllowRebalance()
	}
}
