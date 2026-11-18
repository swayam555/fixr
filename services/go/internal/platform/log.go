package platform

import (
	"log/slog"
	"os"
)

func NewLogger(service string) *slog.Logger {
	level := slog.LevelInfo
	if Env("LOG_LEVEL", "info") == "debug" {
		level = slog.LevelDebug
	}
	l := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})).With("service", service)
	slog.SetDefault(l)
	return l
}
