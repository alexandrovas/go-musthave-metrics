package router

import (
	"net/http"

	"github.com/alexandrovas/go-musthave-metrics/internal/handler"
	"github.com/alexandrovas/go-musthave-metrics/internal/repository"
	"github.com/alexandrovas/go-musthave-metrics/internal/service"
	"github.com/go-chi/chi/v5"
)

func NewRouter() http.Handler {
	repo := repository.NewMemStorage()
	svc := service.NewMetricsService(repo)
	h := handler.NewHandler(svc)

	r := chi.NewRouter()
	r.Post("/update/{type}/{name}/{value}", h.UpdateMetric)

	return r
}
