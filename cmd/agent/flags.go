package main

import (
	"fmt"
	"strconv"
	"time"
)

// durationValue - кастомный тип флага, принимает "30s" и "30" (секунды).
type durationValue time.Duration

func newDurationValue(def time.Duration) *durationValue {
	d := durationValue(def)
	return &d
}

func (d *durationValue) Set(s string) error {
	v, err := time.ParseDuration(s)
	if err != nil {
		secs, err2 := strconv.Atoi(s)
		if err2 != nil {
			return fmt.Errorf("invalid duration %q: use a number of seconds or a string like '30s'", s)
		}
		v = time.Duration(secs) * time.Second
	}
	*d = durationValue(v)
	return nil
}

func (d *durationValue) String() string { return time.Duration(*d).String() }
func (d *durationValue) Type() string   { return "duration" }
