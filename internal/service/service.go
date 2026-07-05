package service

import (
	"errors"

	models "github.com/alexandrovas/go-musthave-metrics/internal/model"
	"github.com/alexandrovas/go-musthave-metrics/internal/repository"
)

var ErrNotFound = errors.New("metric not found")

type MetricsService interface {
	UpdateMetric(metric models.Metrics) error
	GetMetric(mtype models.MetricType, name string) (models.Metrics, error)
	GetAllMetrics() []models.Metrics
}

type metricsService struct {
	repo repository.Repository
}

func NewMetricsService(repo repository.Repository) MetricsService {
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
