package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			s := NewMemStorage()
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
	s := NewMemStorage()
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
			s := NewMemStorage()
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
	s := NewMemStorage()
	_, ok := s.GetCounter("NonExistent")
	assert.False(t, ok)
}

func TestMemStorageAll(t *testing.T) {
	s := NewMemStorage()
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
	s := NewMemStorage()
	s.SetGauge("X", 1)

	cp := s.Gauges()
	cp["X"] = 999 // изменяем копию

	got, _ := s.GetGauge("X")
	assert.Equal(t, 1.0, got, "modifying copy must not affect storage")
}
