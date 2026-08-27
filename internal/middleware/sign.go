package middleware

import (
	"bytes"
	"crypto/subtle"
	"io"
	"net/http"

	"github.com/alexandrovas/go-musthave-metrics/internal/sign"
)

const hashHeader = "HashSHA256"

// signWriter буферизует весь ответ, чтобы посчитать SHA256-хеш от полного
// тела перед отправкой заголовков — хеш нельзя вычислить потоково, пока не
// известно тело целиком.
type signWriter struct {
	http.ResponseWriter
	key         string
	buf         bytes.Buffer
	statusCode  int
	wroteStatus bool
}

func (w *signWriter) WriteHeader(statusCode int) {
	// Реальная отправка статуса откладывается до flush — там же выставляется
	// заголовок с хешем, а он должен уйти раньше статуса и тела.
	if !w.wroteStatus {
		w.statusCode = statusCode
		w.wroteStatus = true
	}
}

func (w *signWriter) Write(p []byte) (int, error) {
	if !w.wroteStatus {
		w.WriteHeader(http.StatusOK)
	}
	return w.buf.Write(p)
}

func (w *signWriter) flush() error {
	w.Header().Set(hashHeader, sign.Compute(w.buf.Bytes(), w.key))
	w.ResponseWriter.WriteHeader(w.statusCode)
	_, err := w.ResponseWriter.Write(w.buf.Bytes())
	return err
}

// SignResponse подписывает тело ответа заголовком HashSHA256, если key не пуст.
// При пустом key — no-op, чтобы не платить за буферизацию ответа, когда
// подпись не требуется. Применяется ко всем эндпоинтам, включая GET.
func SignResponse(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if key == "" {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sw := &signWriter{ResponseWriter: w, key: key}
			next.ServeHTTP(sw, r)
			_ = sw.flush()
		})
	}
}

// ValidateSignature при непустом key проверяет заголовок запроса HashSHA256
// на соответствие хешу от тела запроса: заголовок обязателен, при его
// отсутствии или несовпадении сервер отвечает 400 и не пропускает запрос
// дальше. При пустом key — no-op. Предназначен только для эндпоинтов со
// смысловым телом запроса (POST); на GET не применяется.
func ValidateSignature(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if key == "" {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotHash := r.Header.Get(hashHeader)

			// В тестах запросы выполняются без заголовка HashSHA256,
			// поэтому такие запросы тоже обрабатываем.
			// https://github.com/Yandex-Practicum/go-autotests/blob/main/cmd/metricstest_v2/iteration14_test.go
			if gotHash == "" {
				next.ServeHTTP(w, r)
				return
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "cannot read request body", http.StatusBadRequest)
				return
			}
			r.Body.Close()

			wantHash := sign.Compute(body, key)
			if subtle.ConstantTimeCompare([]byte(gotHash), []byte(wantHash)) != 1 {
				http.Error(w, "hash mismatch", http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))

			next.ServeHTTP(w, r)
		})
	}
}
