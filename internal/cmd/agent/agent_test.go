package agent

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexandrovas/go-musthave-metrics/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var allGaugeNames = []string{
	"Alloc", "BuckHashSys", "Frees", "GCCPUFraction", "GCSys",
	"HeapAlloc", "HeapIdle", "HeapInuse", "HeapObjects", "HeapReleased",
	"HeapSys", "LastGC", "Lookups", "MCacheInuse", "MCacheSys",
	"MSpanInuse", "MSpanSys", "Mallocs", "NextGC", "NumForcedGC",
	"NumGC", "OtherSys", "PauseTotalNs", "StackInuse", "StackSys",
	"Sys", "TotalAlloc", "RandomValue",
}

func TestMetricsStorePoll(t *testing.T) {
	t.Run("all gauges set after one poll", func(t *testing.T) {
		s := &metricsStore{counters: make(map[string]int64), gauges: make(map[string]float64)}
		s.poll()
		s.Lock()
		defer s.Unlock()
		for _, name := range allGaugeNames {
			_, ok := s.gauges[name]
			assert.True(t, ok, "gauge %q not set after poll", name)
		}
	})

	pollCountTests := []struct {
		name   string
		polls  int
		wantPC int64
	}{
		{"single poll", 1, 1},
		{"five polls", 5, 5},
	}
	for _, tc := range pollCountTests {
		t.Run(tc.name, func(t *testing.T) {
			s := &metricsStore{counters: make(map[string]int64), gauges: make(map[string]float64)}
			for range tc.polls {
				s.poll()
			}
			s.Lock()
			got := s.counters["PollCount"]
			s.Unlock()
			require.Equal(t, tc.wantPC, got)
		})
	}
}

func TestSendMetric(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		wantErr bool
	}{
		{"ok", http.StatusOK, false},
		{"bad request", http.StatusBadRequest, true},
		{"not found", http.StatusNotFound, true},
		{"server error", http.StatusInternalServerError, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()

			host := strings.TrimPrefix(srv.URL, "http://")
			a := &agent{
				cfg:        &config.Config{Server: config.ServerConfig{Address: host}},
				httpClient: srv.Client(),
			}
			err := a.sendMetric(t.Context(), "X", "gauge", "1")
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func newTestAgent(t *testing.T, host string, numWorkers uint16, counters map[string]int64, gauges map[string]float64, client *http.Client) *agent {
	t.Helper()
	return &agent{
		cfg: &config.Config{
			Server: config.ServerConfig{Address: host},
			Agent:  config.AgentConfig{Workers: numWorkers},
		},
		metrics:    &metricsStore{counters: counters, gauges: gauges},
		httpClient: client,
		jobs:       make(chan metricValue, 64),
	}
}

func startWorkers(t *testing.T, a *agent, n uint16) *sync.WaitGroup {
	t.Helper()
	var wg sync.WaitGroup
	for idx := range n {
		wg.Go(func() { a.runWorker(t.Context(), idx) })
	}
	return &wg
}

func TestAgentReport(t *testing.T) {
	tests := []struct {
		name      string
		counters  map[string]int64
		gauges    map[string]float64
		wantPaths []string
	}{
		{
			name:      "gauge and counter",
			counters:  map[string]int64{"PollCount": 3},
			gauges:    map[string]float64{"Alloc": 1024.0},
			wantPaths: []string{"/update/gauge/Alloc/1024", "/update/counter/PollCount/3"},
		},
		{
			name:      "float gauge",
			counters:  map[string]int64{},
			gauges:    map[string]float64{"RandomValue": 0.5},
			wantPaths: []string{"/update/gauge/RandomValue/0.5"},
		},
		{
			name:      "counter only",
			counters:  map[string]int64{"PollCount": 7},
			gauges:    map[string]float64{},
			wantPaths: []string{"/update/counter/PollCount/7"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			received := make(map[string]string) // path → Content-Type

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				received[r.URL.Path] = r.Header.Get("Content-Type")
				mu.Unlock()
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			host := strings.TrimPrefix(srv.URL, "http://")
			a := newTestAgent(t, host, 2, tc.counters, tc.gauges, srv.Client())
			wg := startWorkers(t, a, 2)

			a.report(t.Context())
			close(a.jobs)
			wg.Wait()

			mu.Lock()
			defer mu.Unlock()

			for _, path := range tc.wantPaths {
				ct, ok := received[path]
				require.True(t, ok, "expected POST %s", path)
				assert.Equal(t, "text/plain", ct, "Content-Type for %s", path)
			}
		})
	}
}

func TestAgentReportWorkers(t *testing.T) {
	const totalMetrics = 10

	var mu sync.Mutex
	maxConcurrent, current, received := 0, 0, 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		current++
		if current > maxConcurrent {
			maxConcurrent = current
		}
		mu.Unlock()

		time.Sleep(10 * time.Millisecond) // имитируем сетевую задержку

		mu.Lock()
		current--
		received++
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	gauges := make(map[string]float64, totalMetrics)
	for i := range totalMetrics {
		gauges[fmt.Sprintf("gauge%d", i)] = float64(i)
	}

	host := strings.TrimPrefix(srv.URL, "http://")
	const numWorkers = 4
	a := newTestAgent(t, host, numWorkers, make(map[string]int64), gauges, srv.Client())
	wg := startWorkers(t, a, numWorkers)

	a.report(t.Context())
	close(a.jobs)
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	assert.Equal(t, totalMetrics, received, "all metrics must be sent")
	assert.Greater(t, maxConcurrent, 1, "expected parallel requests with %d workers", numWorkers)
}

func TestFormatGaugeValue(t *testing.T) {
	tests := []struct {
		value float64
		want  string
	}{
		{1024, "1024"},
		{1024.5, "1024.5"},
		{0, "0"},
		{0.123456789, "0.123456789"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			got := strconv.FormatFloat(tc.value, 'f', -1, 64)
			assert.Equal(t, tc.want, got)
		})
	}
}
