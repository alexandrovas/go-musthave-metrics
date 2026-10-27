package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		{
			name:  "bare number is treated as seconds",
			input: "30",
			want:  30 * time.Second,
		},
		{
			name:  "bare zero",
			input: "0",
			want:  0,
		},
		{
			name:  "negative number",
			input: "-5",
			want:  -5 * time.Second,
		},
		{
			name:  "explicit seconds unit",
			input: "30s",
			want:  30 * time.Second,
		},
		{
			name:  "fractional seconds",
			input: "1.5s",
			want:  1500 * time.Millisecond,
		},
		{
			name:  "compound duration",
			input: "1h30m",
			want:  90 * time.Minute,
		},
		{
			name:  "milliseconds",
			input: "250ms",
			want:  250 * time.Millisecond,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "not a number and not a duration",
			input:   "abc",
			wantErr: true,
		},
		{
			name:    "mixed invalid unit",
			input:   "30x",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDuration(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, time.Duration(0), got)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
