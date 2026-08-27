package collector

import (
	"fmt"
	"log/slog"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

// Gopsutil собирает системные метрики через gopsutil
type Gopsutil struct {
	*baseCollector
}

func NewGopsutil(logger *slog.Logger) *Gopsutil {
	return &Gopsutil{
		baseCollector: newBaseCollector(logger),
	}
}

func (s *Gopsutil) Poll() {
	vm, err := mem.VirtualMemory()
	if err != nil {
		s.logger.Error("read virtual memory", "error", err)
	} else {
		s.mu.Lock()
		s.gauges["TotalMemory"] = float64(vm.Total)
		s.gauges["FreeMemory"] = float64(vm.Free)
		s.mu.Unlock()
	}

	// interval=0 — не блокируемся: используем дельту с прошлого вызова.
	percents, err := cpu.Percent(0, true)
	if err != nil {
		s.logger.Error("read cpu utilization", "error", err)
		return
	}

	s.mu.Lock()
	for i, p := range percents {
		s.gauges[fmt.Sprintf("CPUutilization%d", i+1)] = p
	}
	s.mu.Unlock()
}

func (s *Gopsutil) Collect() []PendingMetric {
	return s.drain()
}
