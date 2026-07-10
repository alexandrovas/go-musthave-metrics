package repository

import (
	"log/slog"
	"maps"
	"sync"
)

type MemStorage struct {
	gauges   map[string]float64
	counters map[string]int64
	mu       sync.Mutex
}

func NewMemStorage() *MemStorage {
	return &MemStorage{
		gauges:   make(map[string]float64),
		counters: make(map[string]int64),
	}
}

func (s *MemStorage) SetGauge(name string, value float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.gauges[name] = value
}

func (s *MemStorage) GetGauge(name string) (float64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	v, ok := s.gauges[name]
	return v, ok
}

func (s *MemStorage) AddCounter(name string, delta int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.counters[name] += delta
}

func (s *MemStorage) GetCounter(name string) (int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	v, ok := s.counters[name]
	return v, ok
}

// Gauges возвращает копию всех gauge-метрик.
func (s *MemStorage) Gauges() map[string]float64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	cp := make(map[string]float64, len(s.gauges))
	maps.Copy(cp, s.gauges)
	return cp
}

// Counters возвращает копию всех counter-метрик.
func (s *MemStorage) Counters() map[string]int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	cp := make(map[string]int64, len(s.counters))
	maps.Copy(cp, s.counters)
	return cp
}

// Служебная функция для вывода в консоль текущего состояния хранилища
func (s *MemStorage) Log() {
	s.mu.Lock()
	defer s.mu.Unlock()

	slog.Debug("storage state", "gauges", s.gauges, "counters", s.counters)
}
