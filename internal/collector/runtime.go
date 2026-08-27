package collector

import (
	"log/slog"
	"math/rand/v2"
	"runtime"
)

// Runtime собирает runtime-метрики Go
type Runtime struct {
	*baseCollector
}

func NewRuntime(logger *slog.Logger) *Runtime {
	return &Runtime{
		baseCollector: newBaseCollector(logger),
	}
}

func (s *Runtime) Poll() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	s.mu.Lock()
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
	s.mu.Unlock()
}

func (s *Runtime) Collect() []PendingMetric {
	return s.drain()
}
