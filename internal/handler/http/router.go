package handler

import (
	"context"
	"crypto/rsa"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/alexandrovas/go-musthave-metrics/internal/audit"
	"github.com/alexandrovas/go-musthave-metrics/internal/middleware"
	"github.com/alexandrovas/go-musthave-metrics/internal/models"
	"github.com/alexandrovas/go-musthave-metrics/internal/service"
)

// MetricsService — интерфейс бизнес-логики, используемый обработчиками.
type MetricsService interface {
	UpdateMetric(ctx context.Context, metric models.Metrics) error
	UpdateMetrics(ctx context.Context, metric []models.Metrics) error
	GetMetric(ctx context.Context, mtype models.MetricType, name string) (models.Metrics, error)
	GetAllMetrics(ctx context.Context) ([]models.Metrics, error)
	Ping(ctx context.Context) error
}

// Компиляционная проверка: *service.MetricsService реализует MetricsService.
var _ MetricsService = (*service.MetricsService)(nil)

type routerOptions struct {
	hashKey   string
	cryptoKey *rsa.PrivateKey
}

// OptionFunc задаёт дополнительную настройку HTTP-роутера.
type OptionFunc func(*routerOptions)

// WithHashKey включает проверку подписи входящих запросов и подпись ответов.
func WithHashKey(key string) OptionFunc {
	return func(options *routerOptions) {
		options.hashKey = key
	}
}

// WithCryptoKey задаёт закрытый RSA-ключ для расшифровки входящих запросов.
func WithCryptoKey(key *rsa.PrivateKey) OptionFunc {
	return func(options *routerOptions) {
		options.cryptoKey = key
	}
}

// NewRouter собирает chi-роутер со всеми эндпоинтами сервера метрик и
// middleware (логирование, сжатие, опционально подпись и проверка хеша).
func NewRouter(repo service.Repository, logger *slog.Logger, auditor *audit.Dispatcher, opts ...OptionFunc) http.Handler {
	options := routerOptions{}
	for _, optFunc := range opts {
		if optFunc != nil {
			optFunc(&options)
		}
	}

	svc := service.NewMetricsService(repo)
	h := NewHandler(svc, logger, auditor)

	r := chi.NewRouter()

	// Расшифровка оборачивает сжатие: агент шифрует уже gzip-сжатое тело.
	if options.cryptoKey != nil {
		r.Use(middleware.DecryptBody(options.cryptoKey))
	}
	// Глобальные middlewares для всех запросов (логирование и сжатие ответов)
	r.Use(middleware.Logger(logger), middleware.Compression)

	// Если указан hashKey, то подписываем все исходящие ответы
	// и проверяем входящие запросы, если в них есть подпись
	if options.hashKey != "" {
		r.Use(
			middleware.SignResponse(options.hashKey),
			middleware.ValidateSignature(options.hashKey),
		)
	}

	jsonRouter := r.With(middleware.RequireContentTypeJSON)
	jsonRouter.Post("/update", h.UpdateMetricJSON)
	jsonRouter.Post("/update/", h.UpdateMetricJSON) // для успешного прохождения тестов в Github
	jsonRouter.Post("/updates", h.BatchUpdateMeticsJSON)
	jsonRouter.Post("/updates/", h.BatchUpdateMeticsJSON) // для успешного прохождения тестов в Github
	r.Post("/update/{type}/{name}/{value}", h.UpdateMetric)

	jsonRouter.Post("/value", h.ValueJSON)
	jsonRouter.Post("/value/", h.ValueJSON) // для успешного прохождения тестов в Github
	r.Get("/value/{type}/{name}", h.GetMetric)

	r.Get("/ping", h.Ping)

	r.Get("/", h.ListMetrics)

	return r
}
