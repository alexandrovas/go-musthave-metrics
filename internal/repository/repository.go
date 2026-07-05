package repository

import (
	"log/slog"
	"maps"
	"sync"
)

type Repository interface {
	SetGauge(name string, value float64)
	GetGauge(name string) (float64, bool)
	AddCounter(name string, delta int64)
	GetCounter(name string) (int64, bool)
	Gauges() map[string]float64
	Counters() map[string]int64
}

type MemStorage struct {
	gauges   map[string]float64
	counters map[string]int64
	sync.RWMutex
}

func NewMemStorage() *MemStorage {
	return &MemStorage{
		gauges:   make(map[string]float64),
		counters: make(map[string]int64),
	}
}

func (s *MemStorage) SetGauge(name string, value float64) {
	s.Lock()
	defer s.Unlock()
	s.gauges[name] = value
}

func (s *MemStorage) GetGauge(name string) (float64, bool) {
	s.RLock()
	defer s.RUnlock()
	v, ok := s.gauges[name]
	return v, ok
}

func (s *MemStorage) AddCounter(name string, delta int64) {
	s.Lock()
	defer s.Unlock()
	s.counters[name] += delta
}

func (s *MemStorage) GetCounter(name string) (int64, bool) {
	s.RLock()
	defer s.RUnlock()
	v, ok := s.counters[name]
	return v, ok
}

// Gauges возвращает копию всех gauge-метрик.
func (s *MemStorage) Gauges() map[string]float64 {
	s.RLock()
	defer s.RUnlock()
	cp := make(map[string]float64, len(s.gauges))
	maps.Copy(cp, s.gauges)
	return cp
}

// Counters возвращает копию всех counter-метрик.
func (s *MemStorage) Counters() map[string]int64 {
	s.RLock()
	defer s.RUnlock()
	cp := make(map[string]int64, len(s.counters))
	maps.Copy(cp, s.counters)
	return cp
}

// Служебная функция для вывода в консоль текущего состояния хранилища
func (s *MemStorage) Log() {
	s.RLock()
	defer s.RUnlock()
	slog.Debug("storage state", "gauges", s.gauges, "counters", s.counters)
}
