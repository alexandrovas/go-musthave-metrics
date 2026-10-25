package collectors

import (
	"strconv"
	"testing"
)

func BenchmarkBaseCollector_Drain(b *testing.B) {
	c := newBaseCollector(testLogger)
	for i := range 1000 {
		c.gauges["g"+strconv.Itoa(i)] = float64(i)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = c.drain()
	}
}

func BenchmarkRuntime_Poll(b *testing.B) {
	r := NewRuntime(testLogger)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		r.Poll()
	}
}
