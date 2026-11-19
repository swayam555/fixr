package main

import (
	"log/slog"

	"fixr.in/svc/internal/platform"
	"fixr.in/svc/internal/rzplocal"
)

func main() {
	platform.NewLogger("razorpay-local")
	ctx, cancel := platform.SignalContext()
	defer cancel()
	s := rzplocal.New(platform.Env("RAZORPAY_KEY_ID", "rzp_test_local"), platform.Env("RAZORPAY_KEY_SECRET", "local-key-secret"),
		platform.Env("RAZORPAY_WEBHOOK_SECRET", "local-webhook-secret"), platform.Env("WEBHOOK_URL", "http://localhost:8096/pay/webhook"))
	s.Duplicate = platform.Env("DUPLICATE_WEBHOOKS", "false") == "true"
	if err := platform.Run(ctx, platform.Env("ADDR", ":8097"), s.Routes()); err != nil {
		slog.Error("server", "err", err)
	}
}
