package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// responseWriter оборачивает http.ResponseWriter, чтобы перехватить статус-код ответа.
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// Logger логирует каждый HTTP-запрос через slog: метод, путь, статус и время выполнения.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rw, r)

		msg := "request"
		log := slog.With(
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration", time.Since(start))

		switch {
		case rw.status >= 400 && rw.status < 500:
			log.Debug(msg)
		case rw.status >= 500:
			log.Error(msg)
		default:
			log.Info(msg)
		}
	})
}
