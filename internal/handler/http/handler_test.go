package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	models "github.com/alexandrovas/go-musthave-metrics/internal/models"
	"github.com/alexandrovas/go-musthave-metrics/internal/repository"
)

var testLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	repo := repository.NewMemStorage(testLogger)
	srv := httptest.NewServer(NewRouter(repo, testLogger))
	t.Cleanup(srv.Close)
	return srv
}

func request(t *testing.T, srv *httptest.Server, method, path string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp.StatusCode, strings.TrimSpace(string(body))
}

func jsonRequest(t *testing.T, srv *httptest.Server, method, path string, body any) (int, string) {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, srv.URL+path, bodyReader)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp.StatusCode, strings.TrimSpace(string(respBody))
}

func TestUpdateMetric(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		// корректные запросы
		{"gauge integer", http.MethodPost, "/update/gauge/Alloc/1024", http.StatusOK},
		{"gauge float", http.MethodPost, "/update/gauge/GCCPUFraction/0.001", http.StatusOK},
		{"gauge negative", http.MethodPost, "/update/gauge/Temp/-3.14", http.StatusOK},
		{"counter positive", http.MethodPost, "/update/counter/PollCount/42", http.StatusOK},
		{"counter zero", http.MethodPost, "/update/counter/PollCount/0", http.StatusOK},

		// некорректный тип метрики
		{"unknown type", http.MethodPost, "/update/unknown/X/1", http.StatusBadRequest},

		// некорректные значения
		{"gauge non-numeric", http.MethodPost, "/update/gauge/Alloc/abc", http.StatusBadRequest},
		{"counter float as value", http.MethodPost, "/update/counter/PollCount/1.5", http.StatusBadRequest},
		{"counter non-numeric", http.MethodPost, "/update/counter/PollCount/xyz", http.StatusBadRequest},

		// неверный HTTP-метод
		{"GET not allowed", http.MethodGet, "/update/gauge/Alloc/1", http.StatusMethodNotAllowed},
		{"PUT not allowed", http.MethodPut, "/update/gauge/Alloc/1", http.StatusMethodNotAllowed},
	}

	srv := newTestServer(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, _ := request(t, srv, tc.method, tc.path)
			assert.Equal(t, tc.wantStatus, status)
		})
	}
}

func TestUpdateMetricAccumulation(t *testing.T) {
	srv := newTestServer(t)

	status, _ := request(t, srv, http.MethodPost, "/update/counter/PollCount/5")
	require.Equal(t, http.StatusOK, status)
	status, _ = request(t, srv, http.MethodPost, "/update/counter/PollCount/3")
	require.Equal(t, http.StatusOK, status)

	status, _ = request(t, srv, http.MethodPost, "/update/gauge/Alloc/1024")
	require.Equal(t, http.StatusOK, status)
	status, _ = request(t, srv, http.MethodPost, "/update/gauge/Alloc/2048")
	require.Equal(t, http.StatusOK, status)
}

func TestGetMetric(t *testing.T) {
	srv := newTestServer(t)

	request(t, srv, http.MethodPost, "/update/gauge/Alloc/1024")
	request(t, srv, http.MethodPost, "/update/counter/PollCount/5")
	request(t, srv, http.MethodPost, "/update/counter/PollCount/3")

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{"gauge exists", "/value/gauge/Alloc", http.StatusOK, "1024"},
		{"counter accumulated", "/value/counter/PollCount", http.StatusOK, "8"},
		{"gauge not found", "/value/gauge/Unknown", http.StatusNotFound, ""},
		{"counter not found", "/value/counter/Unknown", http.StatusNotFound, ""},
		{"unknown type", "/value/unknown/Alloc", http.StatusBadRequest, ""},
		{"POST not allowed", "/value/gauge/Alloc", http.StatusMethodNotAllowed, ""},
	}
	methods := map[string]string{
		"POST not allowed": http.MethodPost,
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			method := http.MethodGet
			if m, ok := methods[tc.name]; ok {
				method = m
			}
			status, body := request(t, srv, method, tc.path)
			assert.Equal(t, tc.wantStatus, status)
			if tc.wantBody != "" {
				assert.Equal(t, tc.wantBody, body)
			}
		})
	}
}

func TestListMetrics(t *testing.T) {
	srv := newTestServer(t)

	t.Run("empty returns HTML", func(t *testing.T) {
		status, _ := request(t, srv, http.MethodGet, "/")
		assert.Equal(t, http.StatusOK, status)
	})

	request(t, srv, http.MethodPost, "/update/gauge/Alloc/1024")
	request(t, srv, http.MethodPost, "/update/counter/PollCount/7")

	t.Run("lists all metrics", func(t *testing.T) {
		status, body := request(t, srv, http.MethodGet, "/")
		assert.Equal(t, http.StatusOK, status)
		assert.Contains(t, body, "<td>Alloc</td>")
		assert.Contains(t, body, "<td>gauge</td>")
		assert.Contains(t, body, "<td>1024</td>")
		assert.Contains(t, body, "<td>PollCount</td>")
		assert.Contains(t, body, "<td>counter</td>")
		assert.Contains(t, body, "<td>7</td>")
	})
}

func TestListMetricsEscapesHTML(t *testing.T) {
	repo := repository.NewMemStorage(testLogger)
	repo.SetGauge(`<script>alert(1)</script>`, 42)

	srv := httptest.NewServer(NewRouter(repo, testLogger))
	defer srv.Close()

	_, body := request(t, srv, http.MethodGet, "/")

	assert.NotContains(t, body, "<script>alert(1)</script>", "raw script tag must be escaped")
	assert.Contains(t, body, "&lt;script&gt;alert(1)&lt;/script&gt;")
}

func ptr[T any](v T) *T { return &v }

func TestUpdateJSON(t *testing.T) {
	tests := []struct {
		name       string
		body       any
		wantStatus int
	}{
		{
			name:       "valid gauge",
			body:       models.Metrics{ID: "Alloc", MType: models.Gauge, Value: ptr(1024.0)},
			wantStatus: http.StatusOK,
		},
		{
			name:       "valid counter",
			body:       models.Metrics{ID: "PollCount", MType: models.Counter, Delta: ptr(int64(42))},
			wantStatus: http.StatusOK,
		},
		{
			name:       "gauge without value",
			body:       models.Metrics{ID: "Alloc", MType: models.Gauge},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "counter without delta",
			body:       models.Metrics{ID: "PollCount", MType: models.Counter},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty ID",
			body:       models.Metrics{ID: "", MType: models.Gauge, Value: ptr(1.0)},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "unknown type",
			body:       models.Metrics{ID: "X", MType: "unknown"},
			wantStatus: http.StatusBadRequest,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newTestServer(t)
			status, _ := jsonRequest(t, srv, http.MethodPost, "/update", tc.body)
			assert.Equal(t, tc.wantStatus, status)
		})
	}
}

func TestUpdateJSONWithoutContentType(t *testing.T) {
	srv := newTestServer(t)

	body, err := json.Marshal(models.Metrics{ID: "Alloc", MType: models.Gauge, Value: ptr(1.0)})
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/update", bytes.NewReader(body))
	require.NoError(t, err)
	// Без заголовка Content-Type — middleware должен вернуть 400

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUpdateJSONAccumulatesCounter(t *testing.T) {
	srv := newTestServer(t)

	counter := models.Metrics{ID: "PollCount", MType: models.Counter, Delta: ptr(int64(5))}
	jsonRequest(t, srv, http.MethodPost, "/update", counter)
	counter.Delta = ptr(int64(3))
	jsonRequest(t, srv, http.MethodPost, "/update", counter)

	// Проверяем накопленное значение через GET
	status, body := request(t, srv, http.MethodGet, "/value/counter/PollCount")
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "8", body)
}

func TestUpdateJSONGaugeOverwrites(t *testing.T) {
	srv := newTestServer(t)

	gauge := models.Metrics{ID: "Alloc", MType: models.Gauge, Value: ptr(1024.0)}
	jsonRequest(t, srv, http.MethodPost, "/update", gauge)
	gauge.Value = ptr(2048.0)
	jsonRequest(t, srv, http.MethodPost, "/update", gauge)

	status, body := request(t, srv, http.MethodGet, "/value/gauge/Alloc")
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "2048", body)
}

func TestValueJSON(t *testing.T) {
	srv := newTestServer(t)

	// Заранее добавляем метрики
	jsonRequest(t, srv, http.MethodPost, "/update", models.Metrics{ID: "Alloc", MType: models.Gauge, Value: ptr(1024.0)})
	jsonRequest(t, srv, http.MethodPost, "/update", models.Metrics{ID: "PollCount", MType: models.Counter, Delta: ptr(int64(7))})
	jsonRequest(t, srv, http.MethodPost, "/update", models.Metrics{ID: "PollCount", MType: models.Counter, Delta: ptr(int64(3))})

	tests := []struct {
		name       string
		body       any
		wantStatus int
		wantJSON   any
	}{
		{
			name:       "get gauge",
			body:       models.Metrics{ID: "Alloc", MType: models.Gauge},
			wantStatus: http.StatusOK,
			wantJSON:   models.Metrics{ID: "Alloc", MType: models.Gauge, Value: ptr(1024.0)},
		},
		{
			name:       "get counter",
			body:       models.Metrics{ID: "PollCount", MType: models.Counter},
			wantStatus: http.StatusOK,
			wantJSON:   models.Metrics{ID: "PollCount", MType: models.Counter, Delta: ptr(int64(10))},
		},
		{
			name:       "not found",
			body:       models.Metrics{ID: "Unknown", MType: models.Gauge},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "empty ID",
			body:       models.Metrics{ID: "", MType: models.Gauge},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "unknown type",
			body:       models.Metrics{ID: "X", MType: "unknown"},
			wantStatus: http.StatusBadRequest,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, respBody := jsonRequest(t, srv, http.MethodPost, "/value", tc.body)

			assert.Equal(t, tc.wantStatus, status)
			if status == http.StatusOK {
				var got models.Metrics
				err := json.Unmarshal([]byte(respBody), &got)
				require.NoError(t, err)
				assert.Equal(t, tc.wantJSON, got)
			}
		})
	}
}
