package service

import (
	models "github.com/alexandrovas/go-musthave-metrics/internal/model"
	"github.com/alexandrovas/go-musthave-metrics/internal/repository"
)

type MetricsService interface {
	UpdateMetric(metric models.Metrics) error
}

type metricsService struct {
	repo repository.Repository
}

func NewMetricsService(repo repository.Repository) MetricsService {
	return &metricsService{repo: repo}
}

func (s *metricsService) UpdateMetric(metric models.Metrics) error {
	return s.repo.Update(metric)
}
