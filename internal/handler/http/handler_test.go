package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alexandrovas/go-musthave-metrics/internal/repository"
)

func newTestRouter() http.Handler {
	repo := repository.NewMemStorage()
	r := NewRouter(repo)
	return r
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

	r := newTestRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, srv.URL+tc.path, nil)
			require.NoError(t, err)

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)

			assert.Equal(t, tc.wantStatus, resp.StatusCode)
		})
	}
}

func TestUpdateMetricAccumulation(t *testing.T) {
	r := newTestRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()

	post := func(path string) int {
		req, err := http.NewRequest(http.MethodPost, srv.URL+path, nil)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return resp.StatusCode
	}

	// два последовательных counter-запроса должны оба вернуть 200
	require.Equal(t, http.StatusOK, post("/update/counter/PollCount/5"))
	require.Equal(t, http.StatusOK, post("/update/counter/PollCount/3"))

	// gauge-запрос поверх другого gauge — тоже 200
	require.Equal(t, http.StatusOK, post("/update/gauge/Alloc/1024"))
	require.Equal(t, http.StatusOK, post("/update/gauge/Alloc/2048"))
}
