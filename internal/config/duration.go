package config

import (
	"fmt"
	"strconv"
	"time"
)

// ParseDuration разбирает строку в time.Duration. В отличие от time.ParseDuration,
// поддерживает голые числа как секунды: "30" → 30s, "30s" → 30s.
func ParseDuration(s string) (time.Duration, error) {
	v, err := time.ParseDuration(s)
	if err == nil {
		return v, nil
	}

	secs, err := strconv.Atoi(s)
	if err == nil {
		return time.Duration(secs) * time.Second, nil
	}

	return 0, fmt.Errorf("invalid duration %q: use a number of seconds or a string like '30s'", s)
}
