package service

import (
	"context"
	"errors"

	"github.com/alexandrovas/go-musthave-metrics/internal/models"
)

var ErrNotFound = errors.New("metric not found")

type Repository interface {
	SetGauge(ctx context.Context, name string, value float64) error
	GetGauge(ctx context.Context, name string) (float64, bool, error)
	AddCounter(ctx context.Context, name string, delta int64) error
	GetCounter(ctx context.Context, name string) (int64, bool, error)
	Gauges(ctx context.Context) (map[string]float64, error)
	Counters(ctx context.Context) (map[string]int64, error)
	UpdateBatch(ctx context.Context, metric []models.Metrics) error
	Ping(ctx context.Context) error
}

type metricsService struct {
	repo Repository
}

func NewMetricsService(repo Repository) *metricsService {
	return &metricsService{
		repo: repo,
	}
}

func (s *metricsService) UpdateMetrics(ctx context.Context, metric []models.Metrics) error {
	return s.repo.UpdateBatch(ctx, metric)
}

func (s *metricsService) UpdateMetric(ctx context.Context, metric models.Metrics) error {
	switch metric.MType {
	case models.Gauge:
		return s.repo.SetGauge(ctx, metric.ID, *metric.Value)
	case models.Counter:
		return s.repo.AddCounter(ctx, metric.ID, *metric.Delta)
	default:
		return errors.New("unknown metric type")
	}
}

func (s *metricsService) GetMetric(ctx context.Context, mtype models.MetricType, name string) (models.Metrics, error) {
	switch mtype {
	case models.Gauge:
		v, ok, err := s.repo.GetGauge(ctx, name)
		if err != nil {
			return models.Metrics{}, err
		}
		if !ok {
			return models.Metrics{}, ErrNotFound
		}
		return models.Metrics{ID: name, MType: models.Gauge, Value: &v}, nil
	case models.Counter:
		d, ok, err := s.repo.GetCounter(ctx, name)
		if err != nil {
			return models.Metrics{}, err
		}
		if !ok {
			return models.Metrics{}, ErrNotFound
		}
		return models.Metrics{ID: name, MType: models.Counter, Delta: &d}, nil
	}
	return models.Metrics{}, ErrNotFound
}

func (s *metricsService) GetAllMetrics(ctx context.Context) ([]models.Metrics, error) {
	gauges, err := s.repo.Gauges(ctx)
	if err != nil {
		return nil, err
	}
	counters, err := s.repo.Counters(ctx)
	if err != nil {
		return nil, err
	}

	var result []models.Metrics
	for name, v := range gauges {
		result = append(result, models.Metrics{ID: name, MType: models.Gauge, Value: &v})
	}
	for name, v := range counters {
		result = append(result, models.Metrics{ID: name, MType: models.Counter, Delta: &v})
	}
	return result, nil
}

func (s *metricsService) Ping(ctx context.Context) error {
	return s.repo.Ping(ctx)
}
