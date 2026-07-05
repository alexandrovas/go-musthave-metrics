package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	models "github.com/alexandrovas/go-musthave-metrics/internal/model"
	"github.com/alexandrovas/go-musthave-metrics/internal/repository"
)

func ptr[T any](t T) *T {
	return &t
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	repo := repository.NewMemStorage()
	srv := httptest.NewServer(NewRouter(repo))
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
		assert.Contains(t, body, formatMetricRow(
			models.Metrics{
				MType: models.Gauge,
				ID:    "Alloc",
				Value: ptr(float64(1024)),
			}))
		assert.Contains(t, body, formatMetricRow(
			models.Metrics{
				MType: models.Counter,
				ID:    "PollCount",
				Delta: ptr(int64(7)),
			}))
	})
}
