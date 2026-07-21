package helper

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/alexandrovas/go-musthave-metrics/internal/config"
)

func NewLogger(level string, format string) (*slog.Logger, error) {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil {
		return nil, err
	}
	var h slog.Handler
	switch config.LogFormat(format) {
	case config.TextLogFormat:
		h = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: l,
		})
	case config.JsonLogFormat:
		h = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: l,
		})
	default:
		return nil, fmt.Errorf("unexpected log format: %s", format)
	}
	return slog.New(h), nil
}
