package agent

import (
	"math/rand/v2"
	"runtime"
	"sync"

	"github.com/alexandrovas/go-musthave-metrics/internal/models"
)

type collector struct {
	counters map[string]int64
	gauges   map[string]float64
	sync.Mutex
}

func (s *collector) poll() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	s.Lock()
	s.counters["PollCount"] += 1
	s.gauges["RandomValue"] = rand.Float64()
	s.gauges["Alloc"] = float64(m.Alloc)
	s.gauges["BuckHashSys"] = float64(m.BuckHashSys)
	s.gauges["Frees"] = float64(m.Frees)
	s.gauges["GCCPUFraction"] = m.GCCPUFraction
	s.gauges["GCSys"] = float64(m.GCSys)
	s.gauges["HeapAlloc"] = float64(m.HeapAlloc)
	s.gauges["HeapIdle"] = float64(m.HeapIdle)
	s.gauges["HeapInuse"] = float64(m.HeapInuse)
	s.gauges["HeapObjects"] = float64(m.HeapObjects)
	s.gauges["HeapReleased"] = float64(m.HeapReleased)
	s.gauges["HeapSys"] = float64(m.HeapSys)
	s.gauges["LastGC"] = float64(m.LastGC)
	s.gauges["Lookups"] = float64(m.Lookups)
	s.gauges["MCacheInuse"] = float64(m.MCacheInuse)
	s.gauges["MCacheSys"] = float64(m.MCacheSys)
	s.gauges["MSpanInuse"] = float64(m.MSpanInuse)
	s.gauges["MSpanSys"] = float64(m.MSpanSys)
	s.gauges["Mallocs"] = float64(m.Mallocs)
	s.gauges["NextGC"] = float64(m.NextGC)
	s.gauges["NumForcedGC"] = float64(m.NumForcedGC)
	s.gauges["NumGC"] = float64(m.NumGC)
	s.gauges["OtherSys"] = float64(m.OtherSys)
	s.gauges["PauseTotalNs"] = float64(m.PauseTotalNs)
	s.gauges["StackInuse"] = float64(m.StackInuse)
	s.gauges["StackSys"] = float64(m.StackSys)
	s.gauges["Sys"] = float64(m.Sys)
	s.gauges["TotalAlloc"] = float64(m.TotalAlloc)
	s.Unlock()
}

// pendingMetric — метрика, готовая к отправке воркером. Restore нужно вызвать,
// если отправка не удалась: для counter'а это вернёт изъятую дельту обратно
// в накопитель (она уйдёт на следующем цикле report вместе с новыми
// накоплениями), для gauge'а — no-op, так как мгновенное значение попросту
// будет заменено следующим poll().
type pendingMetric struct {
	Metric  models.Metrics
	Restore func()
}

var noopRestore = func() {}

// collect снимает снимок всех метрик для отправки. Дельта каждого counter'а
// оптимистично изымается (drain) из накопителя в этот же момент — это
// гарантирует, что два разных вызова collect() никогда не заберут одну и ту
// же дельту дважды, даже если предыдущий цикл report ещё не завершил
// доставку (агент рассылает метрики асинхронно через пул воркеров).
func (s *collector) collect() []pendingMetric {
	s.Lock()
	defer s.Unlock()

	pending := make([]pendingMetric, 0, len(s.gauges)+len(s.counters))
	for name, value := range s.gauges {
		v := value
		pending = append(pending, pendingMetric{
			Metric:  models.Metrics{ID: name, MType: models.Gauge, Value: &v},
			Restore: noopRestore,
		})
	}
	for name, delta := range s.counters {
		d := delta
		pending = append(pending, pendingMetric{
			Metric: models.Metrics{ID: name, MType: models.Counter, Delta: &d},
			Restore: func() {
				s.restoreCounter(name, d)
			},
		})
		s.counters[name] = 0
	}
	return pending
}

func (s *collector) restoreCounter(name string, delta int64) {
	if delta == 0 {
		return
	}
	s.Lock()
	defer s.Unlock()
	s.counters[name] += delta
}
