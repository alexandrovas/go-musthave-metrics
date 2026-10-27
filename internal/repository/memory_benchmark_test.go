package repository

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/alexandrovas/go-musthave-metrics/internal/models"
)

// newPopulatedStorage создаёт хранилище с n gauge- и n counter-метриками.
func newPopulatedStorage(n int) *MemStorage {
	s := NewMemStorage(testLogger)
	metrics := make([]models.Metrics, 0, 2*n)
	for i := range n {
		g := float64(i)
		d := int64(i)
		metrics = append(metrics,
			models.Metrics{ID: "g" + strconv.Itoa(i), MType: models.Gauge, Value: &g},
			models.Metrics{ID: "c" + strconv.Itoa(i), MType: models.Counter, Delta: &d},
		)
	}
	_ = s.UpdateBatch(context.Background(), metrics)
	return s
}

func BenchmarkMemStorage_UpdateBatch(b *testing.B) {
	const n = 1000
	s := NewMemStorage(testLogger)
	metrics := make([]models.Metrics, n)
	for i := range n {
		g := float64(i)
		metrics[i] = models.Metrics{ID: "g" + strconv.Itoa(i), MType: models.Gauge, Value: &g}
	}

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := s.UpdateBatch(ctx, metrics); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMemStorage_Gauges(b *testing.B) {
	s := newPopulatedStorage(10000)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := s.Gauges(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMemStorage_Save(b *testing.B) {
	s := newPopulatedStorage(1000)
	path := filepath.Join(b.TempDir(), "metrics.json")

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := s.Save(path); err != nil {
			b.Fatal(err)
		}
	}
}
