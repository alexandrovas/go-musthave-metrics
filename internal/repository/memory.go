package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"sync"
	"time"

	"github.com/alexandrovas/go-musthave-metrics/internal/models"
)

type MemStorage struct {
	gauges   map[string]float64
	counters map[string]int64
	mu       sync.Mutex

	syncPath string // если не пусто — синхронное сохранение после каждой мутации
	logger   *slog.Logger
}

func NewMemStorage(logger *slog.Logger) *MemStorage {
	return &MemStorage{
		gauges:   make(map[string]float64),
		counters: make(map[string]int64),
		logger:   logger,
	}
}

// EnableSyncSave включает синхронное сохранение на диск после каждого SetGauge/AddCounter.
func (s *MemStorage) EnableSyncSave(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncPath = path
}

func (s *MemStorage) SetGauge(ctx context.Context, name string, value float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.gauges[name] = value

	if s.syncPath != "" {
		if err := s.saveLocked(s.syncPath); err != nil {
			s.logger.Error("sync save after SetGauge", "error", err)
		}
	}
	return nil
}

func (s *MemStorage) GetGauge(ctx context.Context, name string) (float64, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	v, ok := s.gauges[name]
	return v, ok, nil
}

func (s *MemStorage) AddCounter(ctx context.Context, name string, delta int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.counters[name] += delta

	if s.syncPath != "" {
		if err := s.saveLocked(s.syncPath); err != nil {
			s.logger.Error("sync save after AddCounter", "error", err)
		}
	}
	return nil
}

func (s *MemStorage) GetCounter(ctx context.Context, name string) (int64, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	v, ok := s.counters[name]
	return v, ok, nil
}

// Gauges возвращает копию всех gauge-метрик.
func (s *MemStorage) Gauges(ctx context.Context) (map[string]float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cp := make(map[string]float64, len(s.gauges))
	maps.Copy(cp, s.gauges)
	return cp, nil
}

// Counters возвращает копию всех counter-метрик.
func (s *MemStorage) Counters(ctx context.Context) (map[string]int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cp := make(map[string]int64, len(s.counters))
	maps.Copy(cp, s.counters)
	return cp, nil
}

// Save сериализует все метрики в JSON-файл. Запись атомарная: сначала пишем
// во временный файл, затем делаем rename.
func (s *MemStorage) Save(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(path)
}

// saveLocked — внутренняя версия Save без захвата мьютекса.
func (s *MemStorage) saveLocked(path string) error {
	metrics := make([]models.Metrics, 0, len(s.gauges)+len(s.counters))
	for name, v := range s.gauges {
		metrics = append(metrics, models.Metrics{ID: name, MType: models.Gauge, Value: &v})
	}
	for name, d := range s.counters {
		metrics = append(metrics, models.Metrics{ID: name, MType: models.Counter, Delta: &d})
	}

	// пишем сначала во временный файл
	tmpPath := path + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")

	// если произошла ошибка при записи, удаляем временный файл и возвращаем ошибку
	if err := enc.Encode(metrics); err != nil {
		f.Close()
		s.removeFile(tmpPath)
		return fmt.Errorf("write file: %w", err)
	}

	if err := f.Close(); err != nil {
		s.removeFile(tmpPath)
		return fmt.Errorf("close file: %w", err)
	}

	// атомарно переименовываем файл
	return os.Rename(tmpPath, path)
}

// removeFile - удаляет файл по заданному пути. Если возникает ошибка,
// игнорируем её и пишем в error log
func (s *MemStorage) removeFile(filename string) {
	if err := os.Remove(filename); err != nil {
		s.logger.Error("cannot delete temporary file", "filename", filename, "error", err)
	}
}

// Load загружает метрики из JSON-файла в хранилище.
// Несуществующий файл ошибкой не считается.
func (s *MemStorage) Load(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	var metrics []models.Metrics
	if err := json.NewDecoder(f).Decode(&metrics); err != nil {
		return fmt.Errorf("json decode: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, m := range metrics {
		switch m.MType {
		case models.Gauge:
			if m.Value != nil {
				s.gauges[m.ID] = *m.Value
			}
		case models.Counter:
			if m.Delta != nil {
				s.counters[m.ID] += *m.Delta
			}
		}
	}
	return nil
}

// RunPeriodicSave запускает цикл: с заданным интервалом сохраняет состояние на диск.
// При отмене контекста выполняет финальное сохранение и выходит.
// Вызывающая сторона должна обернуть вызов в горутину.
func (s *MemStorage) RunPeriodicSave(ctx context.Context, path string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	s.logger.Debug("running periodic save state to file", "path", path, "interval", interval)
	for {
		select {
		case <-ticker.C:
			if err := s.Save(path); err != nil {
				s.logger.Error("periodic save failed", "path", path, "error", err)
			} else {
				s.logger.Debug("state saved", "path", path)
			}
		case <-ctx.Done():
			if err := s.Save(path); err != nil {
				s.logger.Error("final save failed", "path", path, "error", err)
			}
			s.logger.Debug("state saved", "path", path)
			return
		}
	}
}

// Служебная функция для вывода в консоль текущего состояния хранилища
func (s *MemStorage) Log() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logger.Debug("storage state", "gauges", s.gauges, "counters", s.counters)
}

// Ping всегда возвращает nil — in-memory хранилище недоступным не бывает.
func (s *MemStorage) Ping(ctx context.Context) error {
	return nil
}
