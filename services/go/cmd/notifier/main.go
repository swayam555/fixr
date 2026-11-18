package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

	"fixr.in/svc/internal/dispatch"
	"fixr.in/svc/internal/notify"
	"fixr.in/svc/internal/platform"
	"fixr.in/svc/internal/realtime"
	"github.com/twmb/franz-go/pkg/kgo"
)

func main() {
	platform.NewLogger("notifier")
	ctx, cancel := platform.SignalContext()
	defer cancel()

	rdb := platform.NewRedis()
	var sender notify.Sender = notify.LogSender{}
	if f := os.Getenv("FCM_CREDENTIALS_FILE"); f != "" {
		s, err := notify.NewFCMSender(ctx, f)
		if err != nil {
			slog.Error("fcm credentials", "err", err)
			os.Exit(1)
		}
		sender = s
	}
	slog.Info("push sender", "name", sender.Name())
	n := &notify.Notifier{Redis: rdb, Emit: &realtime.Emitter{Redis: rdb}, Sender: sender}

	consumer, err := platform.NewConsumer("notifier", []string{"order.events", dispatch.TopicOffers})
	if err != nil {
		slog.Error("kafka", "err", err)
		os.Exit(1)
	}
	defer consumer.Close()
	go func() { _ = platform.Run(ctx, platform.Env("ADDR", ":8095"), http.NewServeMux()) }()

	for ctx.Err() == nil {
		fetches := consumer.PollRecords(ctx, 500)
		failed := false
		fetches.EachRecord(func(r *kgo.Record) {
			var err error
			switch r.Topic {
			case "order.events":
				var ev dispatch.OrderEvent
				if json.Unmarshal(r.Value, &ev) == nil {
					err = n.OnOrderEvent(ctx, ev)
				}
			case dispatch.TopicOffers:
				var ev dispatch.OfferEvent
				if json.Unmarshal(r.Value, &ev) == nil {
					err = n.OnOfferEvent(ctx, ev)
				}
			}
			if err != nil {
				slog.Error("notify", "topic", r.Topic, "err", err)
				failed = true
			}
		})
		if failed {
			time.Sleep(time.Second)
		} else {
			_ = consumer.CommitUncommittedOffsets(context.Background())
		}
		consumer.AllowRebalance()
	}
}
