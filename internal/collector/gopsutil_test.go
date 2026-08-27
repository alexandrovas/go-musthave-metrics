package collector

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGopsutilPoll(t *testing.T) {
	c := NewGopsutil(testLogger)
	c.Poll()

	c.mu.Lock()
	defer c.mu.Unlock()

	_, ok := c.gauges["TotalMemory"]
	assert.True(t, ok, "TotalMemory gauge not set")
	_, ok = c.gauges["FreeMemory"]
	assert.True(t, ok, "FreeMemory gauge not set")

	ncpu := 0
	for name := range c.gauges {
		if strings.HasPrefix(name, "CPUutilization") {
			ncpu++
		}
	}
	assert.Greater(t, ncpu, 0, "expected at least one CPUutilizationN gauge")
	assert.Contains(t, c.gauges, "CPUutilization1", "gauge indices must start at 1")
}
