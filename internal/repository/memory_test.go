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
				s.SetGauge("Alloc", v)
			}
			got, ok := s.GetGauge("Alloc")
			require.True(t, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestMemStorageGaugeNotFound(t *testing.T) {
	s := NewMemStorage(testLogger)
	_, ok := s.GetGauge("NonExistent")
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
				s.AddCounter("PollCount", d)
			}
			got, ok := s.GetCounter("PollCount")
			require.True(t, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestMemStorageCounterNotFound(t *testing.T) {
	s := NewMemStorage(testLogger)
	_, ok := s.GetCounter("NonExistent")
	assert.False(t, ok)
}

func TestMemStorageAll(t *testing.T) {
	s := NewMemStorage(testLogger)
	s.SetGauge("Alloc", 1024)
	s.SetGauge("Sys", 4096)
	s.AddCounter("PollCount", 7)

	gauges := s.Gauges()
	assert.Equal(t, 1024.0, gauges["Alloc"])
	assert.Equal(t, 4096.0, gauges["Sys"])
	assert.Len(t, gauges, 2)

	counters := s.Counters()
	assert.Equal(t, int64(7), counters["PollCount"])
	assert.Len(t, counters, 1)
}

func TestMemStorageGaugesCopy(t *testing.T) {
	s := NewMemStorage(testLogger)
	s.SetGauge("X", 1)

	cp := s.Gauges()
	cp["X"] = 999 // изменяем копию

	got, _ := s.GetGauge("X")
	assert.Equal(t, 1.0, got, "modifying copy must not affect storage")
}

// --- Save / Load tests ---

func TestMemStorageSaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metrics.json")

	src := NewMemStorage(testLogger)
	src.SetGauge("Alloc", 1024.0)
	src.SetGauge("Sys", 4096.0)
	src.AddCounter("PollCount", 7)
	src.AddCounter("PollCount", 3)

	err := src.Save(path)
	require.NoError(t, err)

	dst := NewMemStorage(testLogger)
	err = dst.Load(path)
	require.NoError(t, err)

	v, ok := dst.GetGauge("Alloc")
	require.True(t, ok)
	assert.Equal(t, 1024.0, v)

	v, ok = dst.GetGauge("Sys")
	require.True(t, ok)
	assert.Equal(t, 4096.0, v)

	d, ok := dst.GetCounter("PollCount")
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

	assert.Len(t, dst.Gauges(), 0)
	assert.Len(t, dst.Counters(), 0)
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
	s.SetGauge("Alloc", 1024.0)
	s.AddCounter("PollCount", 42)
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
