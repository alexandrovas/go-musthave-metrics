package handler_test

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"

	handler "github.com/alexandrovas/go-musthave-metrics/internal/handler/http"
	"github.com/alexandrovas/go-musthave-metrics/internal/repository"
)

// newTestRouter собирает роутер с in-memory хранилищем и «молчаливым» логером,
// чтобы примеры не засоряли stdout.
func newTestRouter() http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return handler.NewRouter(repository.NewMemStorage(logger), logger, "", nil)
}

func Example_update() {
	router := newTestRouter()

	// Обновляем gauge-метрику через URL-эндпоинт.
	req := httptest.NewRequest(http.MethodPost, "/update/gauge/Alloc/42.5", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	fmt.Println("update status:", rec.Code)

	// Читаем значение обратно в текстовом виде.
	req = httptest.NewRequest(http.MethodGet, "/value/gauge/Alloc", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	fmt.Println("value:", rec.Body.String())

	// Output:
	// update status: 200
	// value: 42.5
}

func Example_updateJSON() {
	router := newTestRouter()

	// Обновляем counter-метрику через JSON-эндпоинт.
	req := httptest.NewRequest(http.MethodPost, "/update",
		strings.NewReader(`{"id":"PollCount","type":"counter","delta":5}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	fmt.Println("update status:", rec.Code)

	// Читаем значение через JSON-эндпоинт.
	req = httptest.NewRequest(http.MethodPost, "/value",
		strings.NewReader(`{"id":"PollCount","type":"counter"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	fmt.Println("value:", strings.TrimSpace(rec.Body.String()))

	// Output:
	// update status: 200
	// value: {"id":"PollCount","type":"counter","delta":5}
}

func Example_batchUpdate() {
	router := newTestRouter()

	// Пакетно обновляем несколько метрик одним запросом.
	body := `[
		{"id":"Alloc","type":"gauge","value":1024},
		{"id":"PollCount","type":"counter","delta":3}
	]`
	req := httptest.NewRequest(http.MethodPost, "/updates", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	fmt.Println("batch status:", rec.Code)

	// Проверяем, что gauge-метрика из батча сохранилась.
	req = httptest.NewRequest(http.MethodGet, "/value/gauge/Alloc", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	fmt.Println("Alloc:", rec.Body.String())

	// Output:
	// batch status: 200
	// Alloc: 1024
}

func Example_ping() {
	router := newTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	fmt.Println(rec.Code, rec.Body.String())

	// Output:
	// 200 pong
}

func Example_listMetrics() {
	router := newTestRouter()

	// Добавим одну метрику, чтобы HTML-страница не была пустой.
	req := httptest.NewRequest(http.MethodPost, "/update/gauge/Alloc/42.5", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	fmt.Println(rec.Code, strings.Contains(rec.Body.String(), "<title>Metrics</title>"))

	// Output:
	// 200 true
}
