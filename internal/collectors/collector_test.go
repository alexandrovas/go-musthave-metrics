package collectors

import (
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alexandrovas/go-musthave-metrics/internal/models"
)

var testLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestRestoreCounter(t *testing.T) {
	c := newBaseCollector(testLogger)
	c.counters["PollCount"] = 0

	c.restoreCounter("PollCount", 5)

	c.mu.Lock()
	got := c.counters["PollCount"]
	c.mu.Unlock()

	require.Equal(t, int64(5), got)
}

func TestRestoreCounterZeroIsNoop(t *testing.T) {
	c := newBaseCollector(testLogger)
	c.counters["PollCount"] = 3

	c.restoreCounter("PollCount", 0)

	c.mu.Lock()
	got := c.counters["PollCount"]
	c.mu.Unlock()

	require.Equal(t, int64(3), got)
}

func TestCollectDrainsCounters(t *testing.T) {
	c := newBaseCollector(testLogger)
	c.counters["PollCount"] = 5

	values := c.drain()
	require.Len(t, values, 1)
	require.Equal(t, models.Counter, values[0].Metric.MType)
	require.Equal(t, int64(5), *values[0].Metric.Delta)

	// счётчик должен быть обнулён сразу при снятии снимка
	c.mu.Lock()
	got := c.counters["PollCount"]
	c.mu.Unlock()
	require.Equal(t, int64(0), got)
}

func TestGaugeRestoreIsNoop(t *testing.T) {
	c := newBaseCollector(testLogger)
	c.gauges["Alloc"] = 42

	values := c.drain()
	require.Len(t, values, 1)
	require.Equal(t, models.Gauge, values[0].Metric.MType)

	// Restore для gauge не должен паниковать и не должен ничего менять
	require.NotPanics(t, values[0].Restore)
}
