package helper

import (
	"log/slog"
	"os"

	"github.com/alexandrovas/go-musthave-metrics/internal/config"
)

// NewLogger создаёт slog.Logger с заданным уровнем и форматом (json/text).
// На вход в параметр format должно приходить уже провалидированное значение config.LogFormat,
// поэтому дополнительную проверку тут не выполняем.
func NewLogger(level slog.Level, format config.LogFormat) *slog.Logger {
	var h slog.Handler
	switch format {
	case config.JSONLogFormat:
		h = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			Level: level,
		})
	// по умолчанию пишем логи в текстовом формате
	default:
		h = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: level,
		})
	}
	return slog.New(h)
}
