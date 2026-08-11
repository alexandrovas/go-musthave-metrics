package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"sort"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/alexandrovas/go-musthave-metrics/internal/models"
	"github.com/alexandrovas/go-musthave-metrics/internal/service"
)

type Handler struct {
	service MetricsService
	logger  *slog.Logger
}

func NewHandler(s MetricsService, logger *slog.Logger) *Handler {
	return &Handler{
		service: s,
		logger:  logger,
	}
}

func (h *Handler) writeJsonBody(w http.ResponseWriter, resp any, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger.Debug("error encoding response", "error", err)
	}
}

func decodeMetric(r *http.Request) (models.Metrics, error) {
	var metric models.Metrics
	if err := json.NewDecoder(r.Body).Decode(&metric); err != nil {
		return metric, err
	}
	return metric, nil
}

func (h *Handler) ValueJson(w http.ResponseWriter, r *http.Request) {
	metric, err := decodeMetric(r)
	if err != nil {
		h.writeJsonBody(w, models.ErrorResponse{Error: errJSONDecode},
			http.StatusBadRequest)
		return
	}

	if metric.ID == "" {
		h.writeJsonBody(w, models.ErrorResponse{Error: errMetricIdIsRequired},
			http.StatusBadRequest)
		return
	}

	switch metric.MType {
	case models.Gauge, models.Counter:
		metric, err := h.service.GetMetric(r.Context(), metric.MType, metric.ID)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				h.writeJsonBody(w, models.ErrorResponse{Error: service.ErrNotFound},
					http.StatusNotFound)
				return
			}
			h.logger.Error("get metric error", "error", err)
			h.writeJsonBody(w, models.ErrorResponse{Error: errInternal},
				http.StatusInternalServerError)
			return
		}

		h.writeJsonBody(w, metric, http.StatusOK)
		return
	default:
		h.writeJsonBody(w, models.ErrorResponse{Error: errUnknownMetricType},
			http.StatusBadRequest)
		return
	}
}

func (h *Handler) UpdateMetricJson(w http.ResponseWriter, r *http.Request) {
	metric, err := decodeMetric(r)
	if err != nil {
		h.writeJsonBody(w, models.ErrorResponse{Error: errJSONDecode},
			http.StatusBadRequest)
		return
	}

	if metric.ID == "" {
		h.writeJsonBody(w, models.ErrorResponse{Error: errMetricIdIsRequired},
			http.StatusBadRequest)
		return
	}

	switch metric.MType {
	case models.Gauge:
		if metric.Value == nil {
			h.writeJsonBody(w, models.ErrorResponse{Error: errValueIsRequired},
				http.StatusBadRequest)
			return
		}
	case models.Counter:
		if metric.Delta == nil {
			h.writeJsonBody(w, models.ErrorResponse{Error: errDeltaIsRequired},
				http.StatusBadRequest)
			return
		}
	default:
		h.writeJsonBody(w, models.ErrorResponse{Error: errUnknownMetricType},
			http.StatusBadRequest)
		return
	}

	if err := h.service.UpdateMetric(r.Context(), metric); err != nil {
		h.logger.Error("update metric error", "error", err)
		h.writeJsonBody(w, models.ErrorResponse{Error: errFailedUpdateMetrics},
			http.StatusInternalServerError)
		return
	}

	// возвращаем пустой JSON в случае успеха
	h.writeJsonBody(w, make(map[string]interface{}), http.StatusOK)
}

func (h *Handler) BatchUpdateMeticsJson(w http.ResponseWriter, r *http.Request) {
	var metrics []models.Metrics
	if err := json.NewDecoder(r.Body).Decode(&metrics); err != nil {
		h.writeJsonBody(w, models.ErrorResponse{Error: errJSONDecode},
			http.StatusBadRequest)
		return
	}

	if err := h.service.UpdateMetrics(r.Context(), metrics); err != nil {
		h.logger.Error("update metrics error", "error", err)
		h.writeJsonBody(w, models.ErrorResponse{Error: errFailedUpdateMetrics},
			http.StatusInternalServerError)
		return
	}

	// возвращаем пустой JSON в случае успеха
	h.writeJsonBody(w, make(map[string]interface{}), http.StatusOK)
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

	if err := h.service.UpdateMetric(r.Context(), metric); err != nil {
		h.logger.Error("update metric error", "error", err)
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

	metric, err := h.service.GetMetric(r.Context(), mtype, name)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			http.Error(w, "metric not found", http.StatusNotFound)
			return
		}
		h.logger.Error("get metric error", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, formatValue(metric))
}

// metricRow — данные одной строки таблицы для шаблона, значение уже отформатировано в строку.
type metricRow struct {
	ID    string
	MType models.MetricType
	Value string
}

const metricsPageHTML = `<!DOCTYPE html>
<html>
<head><title>Metrics</title></head>
<body>
<h1>Metrics</h1>
<table>
<tr><th>Name</th><th>Type</th><th>Value</th></tr>
{{range .}}<tr><td>{{.ID}}</td><td>{{.MType}}</td><td>{{.Value}}</td></tr>
{{end}}</table>
</body>
</html>`

var metricsPageTmpl = template.Must(template.New("metrics").Parse(metricsPageHTML))

func (h *Handler) ListMetrics(w http.ResponseWriter, r *http.Request) {
	metrics, err := h.service.GetAllMetrics(r.Context())
	if err != nil {
		h.logger.Error("list metrics error", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	sort.Slice(metrics, func(i, j int) bool {
		if metrics[i].MType != metrics[j].MType {
			return metrics[i].MType < metrics[j].MType
		}
		return metrics[i].ID < metrics[j].ID
	})

	rows := make([]metricRow, len(metrics))
	for i, m := range metrics {
		rows[i] = metricRow{ID: m.ID, MType: m.MType, Value: formatValue(m)}
	}

	var buf bytes.Buffer
	if err := metricsPageTmpl.Execute(&buf, rows); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write(buf.Bytes())
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

func (h *Handler) Ping(w http.ResponseWriter, r *http.Request) {
	err := h.service.Ping(r.Context())
	if err != nil {
		h.logger.Error("ping failed", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("pong"))
}
