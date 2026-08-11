package repository

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alexandrovas/go-musthave-metrics/internal/models"
)

var testLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestMemStorageGauge(t *testing.T) {
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
			s := NewMemStorage(testLogger)
			for _, v := range tc.values {
				require.NoError(t, s.SetGauge(t.Context(), "Alloc", v))
			}
			got, ok, err := s.GetGauge(t.Context(), "Alloc")
			require.NoError(t, err)
			require.True(t, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestMemStorageGaugeNotFound(t *testing.T) {
	s := NewMemStorage(testLogger)
	_, ok, err := s.GetGauge(t.Context(), "NonExistent")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestMemStorageCounter(t *testing.T) {
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
			s := NewMemStorage(testLogger)
			for _, d := range tc.deltas {
				require.NoError(t, s.AddCounter(t.Context(), "PollCount", d))
			}
			got, ok, err := s.GetCounter(t.Context(), "PollCount")
			require.NoError(t, err)
			require.True(t, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestMemStorageCounterNotFound(t *testing.T) {
	s := NewMemStorage(testLogger)
	_, ok, err := s.GetCounter(t.Context(), "NonExistent")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestMemStorageAll(t *testing.T) {
	s := NewMemStorage(testLogger)
	require.NoError(t, s.SetGauge(t.Context(), "Alloc", 1024))
	require.NoError(t, s.SetGauge(t.Context(), "Sys", 4096))
	require.NoError(t, s.AddCounter(t.Context(), "PollCount", 7))

	gauges, err := s.Gauges(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1024.0, gauges["Alloc"])
	assert.Equal(t, 4096.0, gauges["Sys"])
	assert.Len(t, gauges, 2)

	counters, err := s.Counters(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int64(7), counters["PollCount"])
	assert.Len(t, counters, 1)
}

func TestMemStorageGaugesCopy(t *testing.T) {
	s := NewMemStorage(testLogger)
	require.NoError(t, s.SetGauge(t.Context(), "X", 1))

	cp, err := s.Gauges(t.Context())
	require.NoError(t, err)
	cp["X"] = 999 // изменяем копию

	got, _, err := s.GetGauge(t.Context(), "X")
	require.NoError(t, err)
	assert.Equal(t, 1.0, got, "modifying copy must not affect storage")
}

// --- Save / Load tests ---

func TestMemStorageSaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metrics.json")

	src := NewMemStorage(testLogger)
	require.NoError(t, src.SetGauge(t.Context(), "Alloc", 1024.0))
	require.NoError(t, src.SetGauge(t.Context(), "Sys", 4096.0))
	require.NoError(t, src.AddCounter(t.Context(), "PollCount", 7))
	require.NoError(t, src.AddCounter(t.Context(), "PollCount", 3))

	err := src.Save(path)
	require.NoError(t, err)

	dst := NewMemStorage(testLogger)
	err = dst.Load(path)
	require.NoError(t, err)

	v, ok, err := dst.GetGauge(t.Context(), "Alloc")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, 1024.0, v)

	v, ok, err = dst.GetGauge(t.Context(), "Sys")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, 4096.0, v)

	d, ok, err := dst.GetCounter(t.Context(), "PollCount")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, int64(10), d)
}

func TestMemStorageLoadNonExistent(t *testing.T) {
	s := NewMemStorage(testLogger)
	err := s.Load("/tmp/no-such-file-42.json")
	require.NoError(t, err)
}

func TestMemStorageSaveEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")

	s := NewMemStorage(testLogger)
	err := s.Save(path)
	require.NoError(t, err)

	dst := NewMemStorage(testLogger)
	err = dst.Load(path)
	require.NoError(t, err)

	gauges, err := dst.Gauges(t.Context())
	require.NoError(t, err)
	assert.Len(t, gauges, 0)

	counters, err := dst.Counters(t.Context())
	require.NoError(t, err)
	assert.Len(t, counters, 0)
}

func TestMemStorageLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")

	err := os.WriteFile(path, []byte("not json"), 0644)
	require.NoError(t, err)

	s := NewMemStorage(testLogger)
	err = s.Load(path)
	assert.Error(t, err)
}

func TestMemStorageSaveProducesValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metrics.json")

	s := NewMemStorage(testLogger)
	require.NoError(t, s.SetGauge(t.Context(), "Alloc", 1024.0))
	require.NoError(t, s.AddCounter(t.Context(), "PollCount", 42))
	err := s.Save(path)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var metrics []models.Metrics
	err = json.Unmarshal(data, &metrics)
	require.NoError(t, err)

	assert.Len(t, metrics, 2)

	found := false
	for _, m := range metrics {
		if m.ID == "Alloc" && m.MType == models.Gauge {
			assert.Equal(t, 1024.0, *m.Value)
			found = true
		}
	}
	assert.True(t, found, "Alloc gauge not found in saved file")
}
