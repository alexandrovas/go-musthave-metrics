package handler

import (
	"bytes"
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
}

func NewHandler(s MetricsService) *Handler {
	return &Handler{service: s}
}

func (h *Handler) ValueJson(w http.ResponseWriter, r *http.Request) {
	metric, err := decodeMetric(r)
	if err != nil {
		writeJsonBody(w, models.ErrorResponse{Error: errJSONDecode},
			http.StatusBadRequest)
		return
	}

	if metric.ID == "" {
		writeJsonBody(w, models.ErrorResponse{Error: errMetricIdIsRequired},
			http.StatusBadRequest)
		return
	}

	switch metric.MType {
	case models.Gauge, models.Counter:
		metric, err := h.service.GetMetric(metric.MType, metric.ID)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				writeJsonBody(w, models.ErrorResponse{Error: service.ErrNotFound},
					http.StatusNotFound)
				return
			}
			slog.Error("get metric error", "error", err)
			writeJsonBody(w, models.ErrorResponse{Error: errInternal},
				http.StatusInternalServerError)
			return
		}

		writeJsonBody(w, metric, http.StatusOK)
		return
	default:
		writeJsonBody(w, models.ErrorResponse{Error: errUnknownMetricType},
			http.StatusBadRequest)
		return
	}
}

func (h *Handler) UpdateMetricJson(w http.ResponseWriter, r *http.Request) {
	metric, err := decodeMetric(r)
	if err != nil {
		writeJsonBody(w, models.ErrorResponse{Error: errJSONDecode},
			http.StatusBadRequest)
		return
	}

	if metric.ID == "" {
		writeJsonBody(w, models.ErrorResponse{Error: errMetricIdIsRequired},
			http.StatusBadRequest)
		return
	}

	switch metric.MType {
	case models.Gauge:
		if metric.Value == nil {
			writeJsonBody(w, models.ErrorResponse{Error: errValueIsRequired},
				http.StatusBadRequest)
			return
		}
	case models.Counter:
		if metric.Delta == nil {
			writeJsonBody(w, models.ErrorResponse{Error: errDeltaIsRequired},
				http.StatusBadRequest)
			return
		}
	default:
		writeJsonBody(w, models.ErrorResponse{Error: errUnknownMetricType},
			http.StatusBadRequest)
		return
	}

	if err := h.service.UpdateMetric(metric); err != nil {
		writeJsonBody(w, models.ErrorResponse{Error: errFailedUpdateMetrics},
			http.StatusInternalServerError)
		return
	}
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
	metrics := h.service.GetAllMetrics()

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
