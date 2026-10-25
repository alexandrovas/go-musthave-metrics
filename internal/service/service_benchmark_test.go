package service

import (
	"context"
	"strconv"
	"testing"

	"github.com/alexandrovas/go-musthave-metrics/internal/models"
)

// benchRepo — in-memory заглушка Repository для бенчмарков.
// Gauges/Counters возвращают заранее заполненные карты без копирования,
// чтобы измерить именно построение среза в GetAllMetrics.
type benchRepo struct {
	gauges   map[string]float64
	counters map[string]int64
}

func (r *benchRepo) SetGauge(context.Context, string, float64) error      { return nil }
func (r *benchRepo) GetGauge(context.Context, string) (float64, bool, error) {
	return 0, false, nil
}
func (r *benchRepo) AddCounter(context.Context, string, int64) error { return nil }
func (r *benchRepo) GetCounter(context.Context, string) (int64, bool, error) {
	return 0, false, nil
}
func (r *benchRepo) Gauges(context.Context) (map[string]float64, error) {
	return r.gauges, nil
}
func (r *benchRepo) Counters(context.Context) (map[string]int64, error) {
	return r.counters, nil
}
func (r *benchRepo) UpdateBatch(context.Context, []models.Metrics) error { return nil }
func (r *benchRepo) Ping(context.Context) error                         { return nil }

// getAllMetricsSink удерживает результат последней итерации, чтобы профиль
// памяти отражал реально аллоцированные структуры, а не только временные.
var getAllMetricsSink []models.Metrics

func newBenchRepo(n int) *benchRepo {
	gauges := make(map[string]float64, n)
	counters := make(map[string]int64, n)
	for i := range n {
		key := strconv.Itoa(i)
		gauges[key] = float64(i)
		counters[key] = int64(i)
	}
	return &benchRepo{gauges: gauges, counters: counters}
}

func BenchmarkMetricsService_GetAllMetrics(b *testing.B) {
	svc := NewMetricsService(newBenchRepo(50000))
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		var err error
		getAllMetricsSink, err = svc.GetAllMetrics(ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMetricsService_UpdateMetric(b *testing.B) {
	svc := NewMetricsService(newBenchRepo(0))
	ctx := context.Background()

	value := 42.0
	metric := models.Metrics{ID: "Alloc", MType: models.Gauge, Value: &value}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := svc.UpdateMetric(ctx, metric); err != nil {
			b.Fatal(err)
		}
	}
}
