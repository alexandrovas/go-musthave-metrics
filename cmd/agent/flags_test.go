package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDurationValueSet(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		{"30s", 30 * time.Second, false},
		{"30", 30 * time.Second, false},
		{"2m", 2 * time.Minute, false},
		{"0", 0, false},
		{"abc", 0, true},
		{"30x", 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			d := newDurationValue(0)
			err := d.Set(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, time.Duration(*d))
		})
	}
}

func TestDurationValueString(t *testing.T) {
	d := newDurationValue(30 * time.Second)
	assert.Equal(t, "30s", d.String())
}
