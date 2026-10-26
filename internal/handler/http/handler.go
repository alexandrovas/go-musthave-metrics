package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/alexandrovas/go-musthave-metrics/internal/audit"
	"github.com/alexandrovas/go-musthave-metrics/internal/models"
	"github.com/alexandrovas/go-musthave-metrics/internal/service"
)

// Handler — HTTP-обработчики эндпоинтов сервера метрик.
type Handler struct {
	service MetricsService
	logger  *slog.Logger
	auditor *audit.Dispatcher
}

// NewHandler создаёт обработчики эндпоинтов поверх сервиса метрик s. События
// аудита асинхронно доставляются диспетчером auditor.
func NewHandler(s MetricsService, logger *slog.Logger, auditor *audit.Dispatcher) *Handler {
	return &Handler{
		service: s,
		logger:  logger,
		auditor: auditor,
	}
}

// clientIP возвращает IP-адрес клиента без порта.
func clientIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// notifyAudit формирует событие аудита и передаёт его диспетчеру для
// асинхронной доставки, не задерживая ответ клиенту.
func (h *Handler) notifyAudit(r *http.Request, metrics []string) {
	if len(metrics) == 0 {
		return
	}
	h.auditor.Notify(audit.NewEvent(metrics, clientIP(r)))
}

func (h *Handler) writeJSONBody(w http.ResponseWriter, resp any, statusCode int) {
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

// ValueJSON обрабатывает POST /value — возвращает метрику по JSON-запросу.
func (h *Handler) ValueJSON(w http.ResponseWriter, r *http.Request) {
	metric, err := decodeMetric(r)
	if err != nil {
		h.writeJSONBody(w, models.ErrorResponse{Error: errJSONDecode},
			http.StatusBadRequest)
		return
	}

	if metric.ID == "" {
		h.writeJSONBody(w, models.ErrorResponse{Error: errMetricIDIsRequired},
			http.StatusBadRequest)
		return
	}

	switch metric.MType {
	case models.Gauge, models.Counter:
		metric, err := h.service.GetMetric(r.Context(), metric.MType, metric.ID)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				h.writeJSONBody(w, models.ErrorResponse{Error: service.ErrNotFound},
					http.StatusNotFound)
				return
			}
			h.logger.Error("get metric error", "error", err)
			h.writeJSONBody(w, models.ErrorResponse{Error: errInternal},
				http.StatusInternalServerError)
			return
		}

		h.writeJSONBody(w, metric, http.StatusOK)
		return
	default:
		h.writeJSONBody(w, models.ErrorResponse{Error: errUnknownMetricType},
			http.StatusBadRequest)
		return
	}
}

// UpdateMetricJSON обрабатывает POST /update — обновляет метрику по JSON-запросу.
func (h *Handler) UpdateMetricJSON(w http.ResponseWriter, r *http.Request) {
	metric, err := decodeMetric(r)
	if err != nil {
		h.writeJSONBody(w, models.ErrorResponse{Error: errJSONDecode},
			http.StatusBadRequest)
		return
	}

	if metric.ID == "" {
		h.writeJSONBody(w, models.ErrorResponse{Error: errMetricIDIsRequired},
			http.StatusBadRequest)
		return
	}

	switch metric.MType {
	case models.Gauge:
		if metric.Value == nil {
			h.writeJSONBody(w, models.ErrorResponse{Error: errValueIsRequired},
				http.StatusBadRequest)
			return
		}
	case models.Counter:
		if metric.Delta == nil {
			h.writeJSONBody(w, models.ErrorResponse{Error: errDeltaIsRequired},
				http.StatusBadRequest)
			return
		}
	default:
		h.writeJSONBody(w, models.ErrorResponse{Error: errUnknownMetricType},
			http.StatusBadRequest)
		return
	}

	if err := h.service.UpdateMetric(r.Context(), metric); err != nil {
		h.logger.Error("update metric error", "error", err)
		h.writeJSONBody(w, models.ErrorResponse{Error: errFailedUpdateMetrics},
			http.StatusInternalServerError)
		return
	}
	h.notifyAudit(r, []string{metric.ID})

	// возвращаем пустой JSON в случае успеха
	h.writeJSONBody(w, make(map[string]interface{}), http.StatusOK)
}

// BatchUpdateMeticsJSON обрабатывает POST /updates — пакетно обновляет метрики.
func (h *Handler) BatchUpdateMeticsJSON(w http.ResponseWriter, r *http.Request) {
	var metrics []models.Metrics
	if err := json.NewDecoder(r.Body).Decode(&metrics); err != nil {
		h.writeJSONBody(w, models.ErrorResponse{Error: errJSONDecode},
			http.StatusBadRequest)
		return
	}

	if err := h.service.UpdateMetrics(r.Context(), metrics); err != nil {
		h.logger.Error("update metrics error", "error", err)
		h.writeJSONBody(w, models.ErrorResponse{Error: errFailedUpdateMetrics},
			http.StatusInternalServerError)
		return
	}

	names := make([]string, len(metrics))
	for i, m := range metrics {
		names[i] = m.ID
	}
	h.notifyAudit(r, names)

	// возвращаем пустой JSON в случае успеха
	h.writeJSONBody(w, make(map[string]interface{}), http.StatusOK)
}

// UpdateMetric обрабатывает POST /update/{type}/{name}/{value} — обновляет метрику,
// передав тип, имя и значение в пути запроса.
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
	h.notifyAudit(r, []string{metric.ID})

	w.WriteHeader(http.StatusOK)
}

// GetMetric обрабатывает GET /value/{type}/{name} — возвращает значение метрики
// в текстовом виде.
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

// ListMetrics обрабатывает GET / — отдаёт HTML-страницу со списком всех метрик.
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

// Ping обрабатывает GET /ping — проверяет доступность хранилища.
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
