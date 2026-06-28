package repository

import models "github.com/alexandrovas/go-musthave-metrics/internal/model"

type Repository interface {
	Update(metric models.Metrics) error
}

type MemStorage struct {
	gauges   map[string]float64
	counters map[string]int64
}

func NewMemStorage() *MemStorage {
	return &MemStorage{
		gauges:   make(map[string]float64),
		counters: make(map[string]int64),
	}
}

func (s *MemStorage) Update(metric models.Metrics) error {
	switch metric.MType {
	case models.Gauge:
		s.gauges[metric.ID] = *metric.Value
	case models.Counter:
		s.counters[metric.ID] += *metric.Delta
	}
	return nil
}
