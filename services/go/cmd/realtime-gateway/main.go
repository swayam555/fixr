package main

import (
	"log/slog"
	"net/http"
	"os"

	"fixr.in/svc/internal/platform"
	"fixr.in/svc/internal/realtime"
)

func main() {
	platform.NewLogger("realtime-gateway")
	ctx, cancel := platform.SignalContext()
	defer cancel()

	verifier, err := platform.NewVerifier(ctx, platform.Env("JWKS_URL", "http://localhost:8080/.well-known/jwks.json"), platform.Env("JWT_ISSUER", "https://fixr.local"))
	if err != nil {
		slog.Error("jwks", "err", err)
		os.Exit(1)
	}
	rdb := platform.NewRedis()
	hub := realtime.NewHub(ctx, rdb)
	defer hub.Close()

	mux := http.NewServeMux()
	mux.Handle("GET /ws", &realtime.Server{Hub: hub, ACL: &realtime.ACL{Redis: rdb}, Verifier: verifier, Origins: platform.EnvList("WS_ORIGINS", "localhost:*,127.0.0.1:*")})
	if err := platform.Run(ctx, platform.Env("ADDR", ":8090"), mux); err != nil {
		slog.Error("server", "err", err)
	}
}
