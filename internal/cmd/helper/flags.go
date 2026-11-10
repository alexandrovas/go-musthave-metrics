package helper

import (
	"log/slog"
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

// LogLevelValue — кастомный тип флага для уровня логирования. Валидирует
// значение (debug, info, warn, error) уже на этапе разбора флага и сразу
// приводит его к slog.Level, поэтому неверный уровень отклоняется до запуска
// приложения.
type LogLevelValue slog.Level

// NewLogLevelValue создаёт LogLevelValue с начальным значением def.
func NewLogLevelValue(def slog.Level) *LogLevelValue {
	l := LogLevelValue(def)
	return &l
}

// Set реализует pflag.Value: валидирует и сохраняет уровень логирования.
func (l *LogLevelValue) Set(s string) error {
	level, err := config.ParseLogLevel(s)
	if err != nil {
		return err
	}
	*l = LogLevelValue(level)
	return nil
}

// String реализует pflag.Value: возвращает каноническое имя уровня.
func (l *LogLevelValue) String() string { return slog.Level(*l).String() }

// Type реализует pflag.Value: уровень передаётся как строка.
func (l *LogLevelValue) Type() string { return "string" }

// LogFormatValue — кастомный тип флага для формата логирования. Валидирует
// значение (text, json) уже на этапе разбора флага и сразу приводит его к
// config.LogFormat, поэтому неверный формат отклоняется до запуска приложения.
type LogFormatValue config.LogFormat

// NewLogFormatValue создаёт LogFormatValue с начальным значением def.
func NewLogFormatValue(def config.LogFormat) *LogFormatValue {
	f := LogFormatValue(def)
	return &f
}

// Set реализует pflag.Value: валидирует и сохраняет формат логирования.
func (f *LogFormatValue) Set(s string) error {
	format, err := config.ParseLogFormat(s)
	if err != nil {
		return err
	}
	*f = LogFormatValue(format)
	return nil
}

// String реализует pflag.Value: возвращает текущий формат в виде строки.
func (f *LogFormatValue) String() string { return string(*f) }

// Type реализует pflag.Value: формат передаётся как строка.
func (f *LogFormatValue) Type() string { return "string" }
