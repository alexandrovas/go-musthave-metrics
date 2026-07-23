package helper

import (
	"time"

	"github.com/alexandrovas/go-musthave-metrics/internal/config"
)

// durationValue - кастомный тип флага, принимает "30s" и "30" (секунды).
type DurationValue time.Duration

func NewDurationValue(def time.Duration) *DurationValue {
	d := DurationValue(def)
	return &d
}

func (d *DurationValue) Set(s string) error {
	v, err := config.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = DurationValue(v)
	return nil
}

func (d *DurationValue) String() string { return time.Duration(*d).String() }
func (d *DurationValue) Type() string   { return "duration" }
