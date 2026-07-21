package handler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/alexandrovas/go-musthave-metrics/internal/middleware"
	"github.com/alexandrovas/go-musthave-metrics/internal/models"
	"github.com/alexandrovas/go-musthave-metrics/internal/service"
)

type MetricsService interface {
	UpdateMetric(metric models.Metrics) error
	GetMetric(mtype models.MetricType, name string) (models.Metrics, error)
	GetAllMetrics() []models.Metrics
}

func NewRouter(repo service.Repository, logger *slog.Logger) http.Handler {
	svc := service.NewMetricsService(repo)
	h := NewHandler(svc, logger)

	r := chi.NewRouter()
	r.Use(middleware.Logger(logger), middleware.Compression)

	r.With(middleware.RequireContentTypeJson).Post("/update", h.UpdateMetricJson)
	r.With(middleware.RequireContentTypeJson).Post("/update/", h.UpdateMetricJson) // для успешного прохождения тестов в Github
	r.Post("/update/{type}/{name}/{value}", h.UpdateMetric)

	r.With(middleware.RequireContentTypeJson).Post("/value", h.ValueJson)
	r.With(middleware.RequireContentTypeJson).Post("/value/", h.ValueJson) // для успешного прохождения тестов в Github
	r.Get("/value/{type}/{name}", h.GetMetric)

	r.Get("/", h.ListMetrics)

	return r
}
