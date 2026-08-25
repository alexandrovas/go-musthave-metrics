package collector

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var allGaugeNames = []string{
	"Alloc", "BuckHashSys", "Frees", "GCCPUFraction", "GCSys",
	"HeapAlloc", "HeapIdle", "HeapInuse", "HeapObjects", "HeapReleased",
	"HeapSys", "LastGC", "Lookups", "MCacheInuse", "MCacheSys",
	"MSpanInuse", "MSpanSys", "Mallocs", "NextGC", "NumForcedGC",
	"NumGC", "OtherSys", "PauseTotalNs", "StackInuse", "StackSys",
	"Sys", "TotalAlloc", "RandomValue",
}

func TestRuntimePoll(t *testing.T) {
	t.Run("all gauges set after one poll", func(t *testing.T) {
		c := NewRuntime(testLogger)
		c.Poll()
		c.Lock()
		defer c.Unlock()
		for _, name := range allGaugeNames {
			_, ok := c.gauges[name]
			assert.True(t, ok, "gauge %q not set after poll", name)
		}
	})

	pollCountTests := []struct {
		name   string
		polls  int
		wantPC int64
	}{
		{"single poll", 1, 1},
		{"five polls", 5, 5},
	}
	for _, tc := range pollCountTests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewRuntime(testLogger)
			for range tc.polls {
				s.Poll()
			}
			s.Lock()
			got := s.counters["PollCount"]
			s.Unlock()
			require.Equal(t, tc.wantPC, got)
		})
	}
}
