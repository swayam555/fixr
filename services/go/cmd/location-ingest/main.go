package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"fixr.in/svc/internal/ingest"
	"fixr.in/svc/internal/platform"
)

func main() {
	platform.NewLogger("location-ingest")
	ctx := context.Background()

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
	svc := &ingest.Service{Redis: platform.NewRedis(), Kafka: producer, Rules: ingest.DefaultRules, Now: time.Now}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /loc/v1/batch", verifier.RequireRole("PRO", svc.Handler))
	if err := platform.Run(ctx, platform.Env("ADDR", ":8091"), mux, func(context.Context) { producer.Close() }); err != nil {
		slog.Error("server", "err", err)
		os.Exit(1)
	}
}
