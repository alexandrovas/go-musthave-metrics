package helper

import (
	"time"

	"github.com/alexandrovas/go-musthave-metrics/internal/config"
)

// DurationValue — кастомный тип флага, принимает "30s" и "30" (секунды).
type DurationValue time.Duration

// NewDurationValue создаёт DurationValue с начальным значением def.
func NewDurationValue(def time.Duration) *DurationValue {
	d := DurationValue(def)
	return &d
}

// Set реализует pflag.Value: парсит строку в DurationValue.
func (d *DurationValue) Set(s string) error {
	v, err := config.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = DurationValue(v)
	return nil
}

// String реализует pflag.Value: возвращает текущее значение в виде строки.
func (d *DurationValue) String() string { return time.Duration(*d).String() }

// Type реализует pflag.Value: возвращает имя типа флага.
func (d *DurationValue) Type() string { return "duration" }
