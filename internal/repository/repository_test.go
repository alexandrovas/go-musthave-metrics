package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	models "github.com/alexandrovas/go-musthave-metrics/internal/model"
)

func ptr[T any](v T) *T { return &v }

func TestMemStorageUpdateGauge(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		want   float64
	}{
		{"single write", []float64{1.5}, 1.5},
		{"overwrite keeps latest", []float64{1.5, 2.5}, 2.5},
		{"zero value", []float64{0}, 0},
		{"negative value", []float64{-3.14}, -3.14},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewMemStorage()
			for _, v := range tc.values {
				require.NoError(t, s.Update(models.Metrics{
					ID:    "Alloc",
					MType: models.Gauge,
					Value: ptr(v),
				}))
			}
			assert.Equal(t, tc.want, s.gauges["Alloc"])
		})
	}
}

func TestMemStorageUpdateCounter(t *testing.T) {
	tests := []struct {
		name   string
		deltas []int64
		want   int64
	}{
		{"single delta", []int64{5}, 5},
		{"accumulates deltas", []int64{5, 3}, 8},
		{"zero delta", []int64{0}, 0},
		{"negative delta", []int64{10, -3}, 7},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewMemStorage()
			for _, d := range tc.deltas {
				require.NoError(t, s.Update(models.Metrics{
					ID:    "PollCount",
					MType: models.Counter,
					Delta: ptr(d),
				}))
			}
			assert.Equal(t, tc.want, s.counters["PollCount"])
		})
	}
}

func TestMemStorageMultipleMetrics(t *testing.T) {
	s := NewMemStorage()

	require.NoError(t, s.Update(models.Metrics{ID: "Alloc", MType: models.Gauge, Value: ptr(1024.0)}))
	require.NoError(t, s.Update(models.Metrics{ID: "Sys", MType: models.Gauge, Value: ptr(4096.0)}))
	require.NoError(t, s.Update(models.Metrics{ID: "PollCount", MType: models.Counter, Delta: ptr(int64(7))}))

	assert.Equal(t, 1024.0, s.gauges["Alloc"])
	assert.Equal(t, 4096.0, s.gauges["Sys"])
	assert.Equal(t, int64(7), s.counters["PollCount"])
	assert.Len(t, s.gauges, 2)
	assert.Len(t, s.counters, 1)
}
