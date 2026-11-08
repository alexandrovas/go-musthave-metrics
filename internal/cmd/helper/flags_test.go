package helper

import (
	"log/slog"
	"testing"
	"time"

	"github.com/alexandrovas/go-musthave-metrics/internal/config"
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
			d := NewDurationValue(0)
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
	d := NewDurationValue(30 * time.Second)
	assert.Equal(t, "30s", d.String())
}

func TestLogLevelValueSet(t *testing.T) {
	tests := []struct {
		input   string
		want    slog.Level
		wantErr bool
	}{
		{"info", slog.LevelInfo, false},
		{"DEBUG", slog.LevelDebug, false},
		{"warn", slog.LevelWarn, false},
		{"error", slog.LevelError, false},
		{"bogus", 0, true},
		{"", 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			l := NewLogLevelValue(slog.LevelInfo)
			err := l.Set(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, slog.Level(*l))
		})
	}
}

func TestLogLevelValueString(t *testing.T) {
	l := NewLogLevelValue(slog.LevelWarn)
	assert.Equal(t, "WARN", l.String())
}

func TestLogFormatValueSet(t *testing.T) {
	tests := []struct {
		input   string
		want    config.LogFormat
		wantErr bool
	}{
		{"text", config.TextLogFormat, false},
		{"json", config.JSONLogFormat, false},
		{"bogus", "", true},
		{"TEXT", "", true},
		{"", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			f := NewLogFormatValue(config.TextLogFormat)
			err := f.Set(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, config.LogFormat(*f))
		})
	}
}

func TestLogFormatValueString(t *testing.T) {
	f := NewLogFormatValue(config.JSONLogFormat)
	assert.Equal(t, "json", f.String())
}
