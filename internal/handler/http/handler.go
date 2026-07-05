package handler

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

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

	metric := models.Metrics{ID: metricName, MType: metricType}

	switch metricType {
	case models.Gauge:
		v, err := strconv.ParseFloat(metricValue, 64)
		if err != nil {
			http.Error(w, "invalid gauge value", http.StatusBadRequest)
			return
		}
		metric.Value = &v
	case models.Counter:
		d, err := strconv.ParseInt(metricValue, 10, 64)
		if err != nil {
			http.Error(w, "invalid counter value", http.StatusBadRequest)
			return
		}
		metric.Delta = &d
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

func (h *Handler) GetMetric(w http.ResponseWriter, r *http.Request) {
	mtype := models.MetricType(chi.URLParam(r, "type"))
	name := chi.URLParam(r, "name")

	switch mtype {
	case models.Gauge, models.Counter:
	default:
		http.Error(w, "unknown metric type", http.StatusBadRequest)
		return
	}

	metric, err := h.service.GetMetric(mtype, name)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			http.Error(w, "metric not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, formatValue(metric))
}

func (h *Handler) ListMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := h.service.GetAllMetrics()

	sort.Slice(metrics, func(i, j int) bool {
		if metrics[i].MType != metrics[j].MType {
			return metrics[i].MType < metrics[j].MType
		}
		return metrics[i].ID < metrics[j].ID
	})

	var sb strings.Builder
	sb.WriteString("<!DOCTYPE html><html><head><title>Metrics</title></head><body>")
	sb.WriteString("<h1>Metrics</h1><table><tr><th>Name</th><th>Type</th><th>Value</th></tr>")
	for _, m := range metrics {
		sb.WriteString(formatMetricRow(m))
	}
	sb.WriteString("</table></body></html>")

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, sb.String())
}

func formatMetricRow(m models.Metrics) string {
	return fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%s</td></tr>", m.ID, m.MType, formatValue(m))
}

func formatValue(m models.Metrics) string {
	switch m.MType {
	case models.Gauge:
		return strconv.FormatFloat(*m.Value, 'f', -1, 64)
	case models.Counter:
		return strconv.FormatInt(*m.Delta, 10)
	}
	return ""
}
