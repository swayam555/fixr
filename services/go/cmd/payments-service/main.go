package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

	"fixr.in/svc/internal/payments"
	"fixr.in/svc/internal/platform"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kgo"
)

func main() {
	platform.NewLogger("payments-service")
	ctx, cancel := platform.SignalContext()
	defer cancel()
	fail := func(what string, err error) {
		slog.Error(what, "err", err)
		os.Exit(1)
	}

	pool, err := pgxpool.New(ctx, platform.Env("DATABASE_URL", "postgres://fixr:fixr@localhost:5432/fixr?pool_max_conns=8"))
	if err != nil {
		fail("postgres", err)
	}
	defer pool.Close()
	svc := &payments.Service{
		DB:            pool,
		Gateway:       payments.NewRazorpay(platform.Env("RAZORPAY_BASE", "http://localhost:8097"), platform.Env("RAZORPAY_KEY_ID", "rzp_test_local"), os.Getenv("RAZORPAY_KEY_SECRET")),
		KeySecret:     os.Getenv("RAZORPAY_KEY_SECRET"),
		WebhookSecret: os.Getenv("RAZORPAY_WEBHOOK_SECRET"),
		FeeBps:        platform.EnvInt("PLATFORM_FEE_BPS", 2000),
	}
	if svc.KeySecret == "" || svc.WebhookSecret == "" {
		fail("config", errMissingSecrets)
	}
	for attempt := 0; ; attempt++ {
		if err = svc.Migrate(ctx); err == nil || attempt == 20 {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		fail("migrate", err)
	}

	verifier, err := platform.NewVerifier(ctx, platform.Env("JWKS_URL", "http://localhost:8080/.well-known/jwks.json"), platform.Env("JWT_ISSUER", "https://fixr.local"))
	if err != nil {
		fail("jwks", err)
	}
	producer, err := platform.NewProducer()
	if err != nil {
		fail("kafka", err)
	}
	defer producer.Close()
	go svc.PublishOutbox(ctx, producer, platform.EnvDuration("OUTBOX_POLL", 200*time.Millisecond))
	go consumeOrders(ctx, svc)

	mux := http.NewServeMux()
	svc.Routes(mux, verifier)
	if err := platform.Run(ctx, platform.Env("ADDR", ":8096"), mux); err != nil {
		slog.Error("server", "err", err)
	}
}

type configError string

func (e configError) Error() string { return string(e) }

const errMissingSecrets = configError("RAZORPAY_KEY_SECRET and RAZORPAY_WEBHOOK_SECRET are required")

// consumeOrders retries a failing record in place rather than skipping it: losing ORDER_COMPLETED
// would leave a finished job that can never be paid.
func consumeOrders(ctx context.Context, svc *payments.Service) {
	consumer, err := platform.NewConsumer("payments-service", []string{"order.events"})
	if err != nil {
		slog.Error("kafka consumer", "err", err)
		os.Exit(1)
	}
	defer consumer.Close()
	for ctx.Err() == nil {
		fetches := consumer.PollRecords(ctx, 500)
		fetches.EachRecord(func(r *kgo.Record) {
			var ev payments.OrderEvent
			if json.Unmarshal(r.Value, &ev) != nil {
				return
			}
			for backoff := time.Second; ctx.Err() == nil; backoff = min(backoff*2, 30*time.Second) {
				if err := svc.OnOrderEvent(ctx, ev); err == nil {
					return
				} else {
					slog.Error("order event", "order", ev.OrderID, "err", err)
				}
				time.Sleep(backoff)
			}
		})
		_ = consumer.CommitUncommittedOffsets(context.Background())
		consumer.AllowRebalance()
	}
}
