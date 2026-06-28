package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	models "github.com/alexandrovas/go-musthave-metrics/internal/model"
	"github.com/alexandrovas/go-musthave-metrics/internal/service"
)

type Handler struct {
	service service.MetricsService
}

func NewHandler(s service.MetricsService) *Handler {
	return &Handler{service: s}
}

func (h *Handler) UpdateMetric(w http.ResponseWriter, r *http.Request) {
	metricType := models.MetricType(chi.URLParam(r, "type"))
	metricName := chi.URLParam(r, "name")
	metricValue := chi.URLParam(r, "value")

	metric := models.Metrics{
		ID:    metricName,
		MType: metricType,
	}

	switch metricType {
	case models.Gauge:
		v, err := strconv.ParseFloat(metricValue, 64)
		if err != nil {
			http.Error(w, "invalid gauge value", http.StatusBadRequest)
			return
		}
		metric.Value = &v
	case models.Counter:
		v, err := strconv.ParseInt(metricValue, 10, 64)
		if err != nil {
			http.Error(w, "invalid counter value", http.StatusBadRequest)
			return
		}
		metric.Delta = &v
	default:
		http.Error(w, "unknown metric type", http.StatusBadRequest)
		return
	}

	if err := h.service.UpdateMetric(metric); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
