package service

import (
	"errors"

	models "github.com/alexandrovas/go-musthave-metrics/internal/model"
)

var ErrNotFound = errors.New("metric not found")

type Repository interface {
	SetGauge(name string, value float64)
	GetGauge(name string) (float64, bool)
	AddCounter(name string, delta int64)
	GetCounter(name string) (int64, bool)
	Gauges() map[string]float64
	Counters() map[string]int64
}

type metricsService struct {
	repo Repository
}

func NewMetricsService(repo Repository) *metricsService {
	return &metricsService{repo: repo}
}

func (s *metricsService) UpdateMetric(metric models.Metrics) error {
	switch metric.MType {
	case models.Gauge:
		s.repo.SetGauge(metric.ID, *metric.Value)
	case models.Counter:
		s.repo.AddCounter(metric.ID, *metric.Delta)
	default:
		return errors.New("unknown metric type")
	}
	return nil
}

func (s *metricsService) GetMetric(mtype models.MetricType, name string) (models.Metrics, error) {
	switch mtype {
	case models.Gauge:
		v, ok := s.repo.GetGauge(name)
		if !ok {
			return models.Metrics{}, ErrNotFound
		}
		return models.Metrics{ID: name, MType: models.Gauge, Value: &v}, nil
	case models.Counter:
		d, ok := s.repo.GetCounter(name)
		if !ok {
			return models.Metrics{}, ErrNotFound
		}
		return models.Metrics{ID: name, MType: models.Counter, Delta: &d}, nil
	}
	return models.Metrics{}, ErrNotFound
}

func (s *metricsService) GetAllMetrics() []models.Metrics {
	var result []models.Metrics
	for name, v := range s.repo.Gauges() {
		result = append(result, models.Metrics{ID: name, MType: models.Gauge, Value: &v})
	}
	for name, v := range s.repo.Counters() {
		result = append(result, models.Metrics{ID: name, MType: models.Counter, Delta: &v})
	}
	return result
}
