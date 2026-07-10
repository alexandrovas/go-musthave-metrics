package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/alexandrovas/go-musthave-metrics/internal/middleware"
	"github.com/alexandrovas/go-musthave-metrics/internal/service"
)

func NewRouter(repo service.Repository) http.Handler {
	svc := service.NewMetricsService(repo)
	h := NewHandler(svc)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Post("/update/{type}/{name}/{value}", h.UpdateMetric)
	r.Get("/value/{type}/{name}", h.GetMetric)
	r.Get("/", h.ListMetrics)

	return r
}
