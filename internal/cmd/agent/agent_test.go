package agent

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alexandrovas/go-musthave-metrics/internal/config"
	"github.com/alexandrovas/go-musthave-metrics/internal/models"
	"github.com/alexandrovas/go-musthave-metrics/internal/sign"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

var allGaugeNames = []string{
	"Alloc", "BuckHashSys", "Frees", "GCCPUFraction", "GCSys",
	"HeapAlloc", "HeapIdle", "HeapInuse", "HeapObjects", "HeapReleased",
	"HeapSys", "LastGC", "Lookups", "MCacheInuse", "MCacheSys",
	"MSpanInuse", "MSpanSys", "Mallocs", "NextGC", "NumForcedGC",
	"NumGC", "OtherSys", "PauseTotalNs", "StackInuse", "StackSys",
	"Sys", "TotalAlloc", "RandomValue",
}

func ptr[T any](t T) *T {
	return &t
}

func TestMetricsStorePoll(t *testing.T) {
	t.Run("all gauges set after one poll", func(t *testing.T) {
		c := &collector{
			counters: make(map[string]int64),
			gauges:   make(map[string]float64),
		}
		c.poll()
		c.Lock()
		defer c.Unlock()
		for _, name := range allGaugeNames {
			_, ok := c.gauges[name]
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
			s := &collector{counters: make(map[string]int64), gauges: make(map[string]float64)}
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
		metric  models.Metrics
		name    string
		status  int
		wantErr bool
	}{
		{models.Metrics{ID: "counter1", MType: models.Counter, Delta: ptr(int64(100))}, "ok", http.StatusOK, false},
		{models.Metrics{}, "bad request", http.StatusBadRequest, true},
		{models.Metrics{}, "not found", http.StatusNotFound, true},
		{models.Metrics{}, "server error", http.StatusInternalServerError, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()

			host := strings.TrimPrefix(srv.URL, "http://")
			a := &Agent{
				cfg: &config.AgentConfig{
					ServerAddress: host,
				},
				httpClient: srv.Client(),
				logger:     testLogger,
			}
			err := a.sendMetric(t.Context(), tc.metric)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func newTestAgent(t *testing.T, host string, numWorkers uint16, counters map[string]int64, gauges map[string]float64, client *http.Client) *Agent {
	t.Helper()
	return &Agent{
		cfg: &config.AgentConfig{
			ServerAddress:  host,
			Workers:        numWorkers,
			PollInterval:   time.Second * 3,
			ReportInterval: time.Second * 1,
		},
		collector:  &collector{counters: counters, gauges: gauges},
		httpClient: client,
		jobs:       make(chan job, 64),
		logger:     testLogger,
	}
}

func startWorkers(t *testing.T, a *Agent, n uint16) *sync.WaitGroup {
	t.Helper()
	var wg sync.WaitGroup
	for idx := range n {
		wg.Go(func() { a.runWorker(t.Context(), idx) })
	}
	return &wg
}

func TestAgentReport(t *testing.T) {
	tests := []struct {
		name     string
		counters map[string]int64
		gauges   map[string]float64
	}{
		{
			name:     "gauge and counter",
			counters: map[string]int64{"PollCount": 3},
			gauges:   map[string]float64{"Alloc": 1024.0},
		},
		{
			name:     "float gauge",
			counters: map[string]int64{},
			gauges:   map[string]float64{"RandomValue": 0.5},
		},
		{
			name:     "counter only",
			counters: map[string]int64{"PollCount": 7},
			gauges:   map[string]float64{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			received := make([]models.Metrics, 0)

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/update", r.URL.Path)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				assert.Equal(t, "gzip", r.Header.Get("Content-Encoding"))

				zr, err := gzip.NewReader(r.Body)
				require.NoError(t, err)
				defer zr.Close()

				body, err := io.ReadAll(zr)
				require.NoError(t, err)

				var m models.Metrics
				err = json.Unmarshal(body, &m)
				require.NoError(t, err)

				mu.Lock()
				received = append(received, m)
				mu.Unlock()
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			// collect() drains counters, so copy expected values before report()
			wantGauges := make(map[string]float64, len(tc.gauges))
			wantCounters := make(map[string]int64, len(tc.counters))
			for k, v := range tc.gauges {
				wantGauges[k] = v
			}
			for k, v := range tc.counters {
				wantCounters[k] = v
			}
			expectedCount := len(wantGauges) + len(wantCounters)

			host := strings.TrimPrefix(srv.URL, "http://")
			a := newTestAgent(t, host, 2, tc.counters, tc.gauges, srv.Client())
			wg := startWorkers(t, a, 2)

			a.report(t.Context(), false)
			close(a.jobs)
			wg.Wait()

			mu.Lock()
			defer mu.Unlock()

			require.Len(t, received, expectedCount)

			for _, m := range received {
				switch m.MType {
				case models.Gauge:
					want, ok := wantGauges[m.ID]
					require.True(t, ok, "unexpected gauge %q", m.ID)
					assert.Equal(t, want, *m.Value)
					delete(wantGauges, m.ID)
				case models.Counter:
					want, ok := wantCounters[m.ID]
					require.True(t, ok, "unexpected counter %q", m.ID)
					assert.Equal(t, want, *m.Delta)
					delete(wantCounters, m.ID)
				default:
					t.Fatalf("unknown metric type %q", m.MType)
				}
			}
			assert.Empty(t, wantGauges, "not all gauges sent")
			assert.Empty(t, wantCounters, "not all counters sent")
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

	a.report(t.Context(), false)
	close(a.jobs)
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	assert.Equal(t, totalMetrics, received, "all metrics must be sent")
	assert.Greater(t, maxConcurrent, 1, "expected parallel requests with %d workers", numWorkers)
}

func TestCollectorRestoreCounter(t *testing.T) {
	c := &collector{
		counters: map[string]int64{"PollCount": 0},
		gauges:   make(map[string]float64),
	}

	c.restoreCounter("PollCount", 5)

	c.Lock()
	got := c.counters["PollCount"]
	c.Unlock()

	require.Equal(t, int64(5), got)
}

func TestCollectorRestoreCounterZeroIsNoop(t *testing.T) {
	c := &collector{
		counters: map[string]int64{"PollCount": 3},
		gauges:   make(map[string]float64),
	}

	c.restoreCounter("PollCount", 0)

	c.Lock()
	got := c.counters["PollCount"]
	c.Unlock()

	require.Equal(t, int64(3), got)
}

func TestCollectorCollectDrainsCounters(t *testing.T) {
	c := &collector{
		counters: map[string]int64{"PollCount": 5},
		gauges:   make(map[string]float64),
	}

	values := c.collect()
	require.Len(t, values, 1)
	require.Equal(t, models.Counter, values[0].Metric.MType)
	require.Equal(t, int64(5), *values[0].Metric.Delta)

	// счётчик должен быть обнулён сразу при снятии снимка
	c.Lock()
	got := c.counters["PollCount"]
	c.Unlock()
	require.Equal(t, int64(0), got)
}

func TestPendingMetricGaugeRestoreIsNoop(t *testing.T) {
	c := &collector{
		counters: make(map[string]int64),
		gauges:   map[string]float64{"Alloc": 42},
	}

	values := c.collect()
	require.Len(t, values, 1)
	require.Equal(t, models.Gauge, values[0].Metric.MType)

	// Restore для gauge не должен паниковать и не должен ничего менять
	require.NotPanics(t, values[0].Restore)
}

func TestAgentReportRestoresCounterOnFailedSend(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	a := newTestAgent(t, host,
		1,
		map[string]int64{"PollCount": 5},
		make(map[string]float64),
		srv.Client())
	wg := startWorkers(t, a, 1)

	a.report(t.Context(), false)
	close(a.jobs)
	wg.Wait()

	// отправка не удалась (500) — дельта должна вернуться в счётчик, а не потеряться
	a.collector.Lock()
	got := a.collector.counters["PollCount"]
	a.collector.Unlock()
	assert.Equal(t, int64(5), got)
}

// --- retry tests ---

func TestIsRetriableSendError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"connection refused (net.OpError)", &net.OpError{Op: "dial", Err: errors.New("connection refused")}, true},
		{"business error (non-200 status)", fmt.Errorf("unexpected status %s", "500 Internal Server Error"), false},
		{"json encode error", fmt.Errorf("json encode error: %w", errors.New("boom")), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isRetriableSendError(tc.err))
		})
	}
}

// TestAgentReportDoesNotRetryOnBusinessError убеждается, что при неуспешном
// HTTP-статусе (не проблема соединения) агент не выполняет дополнительных
// попыток — retryIntervals заведомо большие, но тест должен завершиться быстро.
func TestAgentReportDoesNotRetryOnBusinessError(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	a := newTestAgent(t, host, 1, map[string]int64{"PollCount": 5}, make(map[string]float64), srv.Client())
	a.retryIntervals = []time.Duration{time.Minute, time.Minute, time.Minute} // огромные — тест зависнет, если retry сработает

	err := a.sendMetric(t.Context(), models.Metrics{ID: "PollCount", MType: models.Counter, Delta: ptr(int64(5))})
	assert.Error(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&attempts), "non-connection errors must not be retried")
}

// TestSendDataRetriesOnConnectionRefused проверяет полный retry-путь: агент не
// может установить соединение (никто не слушает адрес), несколько попыток
// проваливаются, а как только сервер поднимается — очередная попытка успешна.
func TestSendDataRetriesOnConnectionRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close()) // освобождаем порт — пока никто не слушает

	var received int32
	go func() {
		time.Sleep(15 * time.Millisecond) // даём агенту сделать пару неудачных попыток
		ln2, err := net.Listen("tcp", addr)
		if err != nil {
			return
		}
		mux := http.NewServeMux()
		mux.HandleFunc("/update", func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&received, 1)
			w.WriteHeader(http.StatusOK)
		})
		httpSrv := &http.Server{Handler: mux}
		t.Cleanup(func() { httpSrv.Close() })
		_ = httpSrv.Serve(ln2)
	}()

	a := &Agent{
		cfg:            &config.AgentConfig{ServerAddress: addr},
		httpClient:     http.DefaultClient,
		logger:         testLogger,
		retryIntervals: []time.Duration{5 * time.Millisecond, 5 * time.Millisecond, 50 * time.Millisecond},
	}

	err = a.sendMetric(t.Context(), models.Metrics{ID: "x", MType: models.Counter, Delta: ptr(int64(1))})
	require.NoError(t, err, "should eventually succeed once the server starts listening")
	assert.Equal(t, int32(1), atomic.LoadInt32(&received))
}

// --- signing tests ---

func TestSendMetricSetsHashHeaderWhenKeyConfigured(t *testing.T) {
	var gotHash string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHash = r.Header.Get(hashHeader)
		zr, err := gzip.NewReader(r.Body)
		require.NoError(t, err)
		defer zr.Close()
		gotBody, err = io.ReadAll(zr)
		require.NoError(t, err)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	a := &Agent{
		cfg:        &config.AgentConfig{ServerAddress: host, Key: "secret"},
		httpClient: srv.Client(),
		logger:     testLogger,
	}

	metric := models.Metrics{ID: "PollCount", MType: models.Counter, Delta: ptr(int64(5))}
	require.NoError(t, a.sendMetric(t.Context(), metric))

	require.NotEmpty(t, gotHash)
	assert.Equal(t, sign.Compute(gotBody, "secret"), gotHash)
}

func TestSendMetricOmitsHashHeaderWhenNoKey(t *testing.T) {
	var gotHash string
	sawHeader := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHash, sawHeader = r.Header.Get(hashHeader), r.Header.Get(hashHeader) != ""
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	a := &Agent{
		cfg:        &config.AgentConfig{ServerAddress: host},
		httpClient: srv.Client(),
		logger:     testLogger,
	}

	metric := models.Metrics{ID: "PollCount", MType: models.Counter, Delta: ptr(int64(5))}
	require.NoError(t, a.sendMetric(t.Context(), metric))

	assert.False(t, sawHeader, "no key configured — request must not carry a hash header")
	assert.Empty(t, gotHash)
}
