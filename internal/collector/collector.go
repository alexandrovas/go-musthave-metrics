// Package collector содержит коллекторы метрик агента. Каждый коллектор
// независимо опрашивает свой источник (runtime-метрики Go, системные метрики
// через gopsutil и т.д.), накапливает значения во внутреннем хранилище и по
// запросу снимает их снимок для отправки.
package collector

import (
	"log/slog"
	"sync"

	"github.com/alexandrovas/go-musthave-metrics/internal/models"
)

// PendingMetric — метрика, готовая к отправке. Restore нужно вызвать, если отправка
// не удалась: для counter'а это вернёт изъятую дельту обратно в накопитель
// (она уйдёт на следующем цикле report вместе с новыми накоплениями), для
// gauge'а — no-op, так как мгновенное значение попросту будет заменено
// следующим poll().
type PendingMetric struct {
	Metric  models.Metrics
	Restore func()
}

var noopRestore = func() {}

// baseCollector — общее для всех коллекторов: потокобезопасное внутреннее
// хранилище gauges/counters и методы работы с ним. Конкретные коллекторы
// встраивают baseCollector и добавляют собственный Poll(), определяющий, что
// именно собирается, и собственный Collect(), снимающий снимок.
type baseCollector struct {
	counters map[string]int64
	gauges   map[string]float64
	logger   *slog.Logger
	sync.Mutex
}

func newBaseCollector(logger *slog.Logger) *baseCollector {
	return &baseCollector{
		counters: make(map[string]int64),
		gauges:   make(map[string]float64),
		logger:   logger,
	}
}

// restoreCounter возвращает изъятую дельту counter'а обратно в накопитель.
func (s *baseCollector) restoreCounter(name string, delta int64) {
	if delta == 0 {
		return
	}
	s.Lock()
	defer s.Unlock()
	s.counters[name] += delta
}

// drain снимает снимок всех накопленных метрик. Дельта каждого counter'а
// оптимистично изымается (обнуляется) в этот же момент — это гарантирует, что
// два разных вызова не заберут одну и ту же дельту дважды, даже если
// предыдущий цикл report ещё не завершил доставку.
func (s *baseCollector) drain() []PendingMetric {
	s.Lock()
	defer s.Unlock()

	pending := make([]PendingMetric, 0, len(s.gauges)+len(s.counters))
	for name, value := range s.gauges {
		v := value
		pending = append(pending, PendingMetric{
			Metric:  models.Metrics{ID: name, MType: models.Gauge, Value: &v},
			Restore: noopRestore,
		})
	}
	for name, delta := range s.counters {
		d := delta
		pending = append(pending, PendingMetric{
			Metric: models.Metrics{ID: name, MType: models.Counter, Delta: &d},
			Restore: func() {
				s.restoreCounter(name, d)
			},
		})
		s.counters[name] = 0
	}
	return pending
}
