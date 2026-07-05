package agent

import (
	"math/rand/v2"
	"runtime"
	"strconv"
	"sync"

	models "github.com/alexandrovas/go-musthave-metrics/internal/model"
)

type collector struct {
	counters map[string]int64
	gauges   map[string]float64
	sync.Mutex
}

type metricValue struct {
	name  string
	value string
	mtype models.MetricType
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

func (s *collector) collect() []metricValue {
	s.Lock()
	defer s.Unlock()

	values := make([]metricValue, 0, len(s.gauges)+len(s.counters))
	for name, value := range s.gauges {
		values = append(values, metricValue{
			name:  name,
			value: strconv.FormatFloat(value, 'f', -1, 64),
			mtype: models.Gauge,
		})
	}
	for name, value := range s.counters {
		values = append(values, metricValue{
			name:  name,
			value: strconv.FormatInt(value, 10),
			mtype: models.Counter,
		})
	}
	// reset counters
	s.counters["PollCount"] = 0
	return values
}
