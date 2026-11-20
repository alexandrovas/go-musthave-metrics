package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/koanf/v2"
	"github.com/spf13/pflag"
)

// LogFormat — формат логов.
type LogFormat string

const (
	// JSONLogFormat — логи в формате JSON.
	JSONLogFormat LogFormat = "json"
	// TextLogFormat — логи в текстовом формате.
	TextLogFormat LogFormat = "text"
)

// ParseLogFormat валидирует строку и возвращает формат логирования.
func ParseLogFormat(s string) (LogFormat, error) {
	f := LogFormat(s)
	switch f {
	case TextLogFormat, JSONLogFormat:
		return f, nil
	default:
		return "", fmt.Errorf("invalid log format %q: expected one of %q", s, []LogFormat{TextLogFormat, JSONLogFormat})
	}
}

// ParseLogLevel валидирует строку и возвращает уровень логирования.
func ParseLogLevel(s string) (slog.Level, error) {
	var l slog.Level
	if err := l.UnmarshalText([]byte(s)); err != nil {
		return 0, fmt.Errorf("invalid log level %q: expected one of %q", s, []string{"debug", "info", "warn", "error"})
	}
	return l, nil
}

// LogConfig — настройки логирования (уровень и формат).
type LogConfig struct {
	Level  slog.Level `koanf:"level"`
	Format LogFormat  `koanf:"format"`
}

// ServerConfig — конфигурация сервера.
type ServerConfig struct {
	Address         string        `koanf:"address"`
	Log             LogConfig     `koanf:"log"`
	StoreInterval   time.Duration `koanf:"store_interval"`
	FileStoragePath string        `koanf:"file_storage_path"`
	RestoreState    bool          `koanf:"restore"`
	DatabaseDSN     string        `koanf:"database_dsn"`
	Key             string        `koanf:"key"`
	CryptoKey       string        `koanf:"crypto_key"`
	AuditFile       string        `koanf:"audit_file"`
	AuditURL        string        `koanf:"audit_url"`
	AuditWorkers    int           `koanf:"audit_workers"`
	AuditQueueSize  int           `koanf:"audit_queue_size"`
}

// AgentConfig — конфигурация агента сбора метрик.
type AgentConfig struct {
	ServerAddress  string        `koanf:"address"`
	PollInterval   time.Duration `koanf:"poll_interval"`
	ReportInterval time.Duration `koanf:"report_interval"`
	RateLimit      uint16        `koanf:"rate_limit"`
	Log            LogConfig     `koanf:"log"`
	Key            string        `koanf:"key"`
	CryptoKey      string        `koanf:"crypto_key"`
}

// durationDecodeHook учит mapstructure понимать голые числа как секунды.
func durationDecodeHook(from reflect.Type, to reflect.Type, data any) (any, error) {
	if from.Kind() != reflect.String || to != reflect.TypeFor[time.Duration]() {
		return data, nil
	}
	d, err := ParseDuration(data.(string))
	if err != nil {
		return data, err
	}
	return d, nil
}

// slogLevelDecodeHook валидирует уровень логирования, приходящий строкой из
// файла, переменных окружения или флагов.
func slogLevelDecodeHook(from reflect.Type, to reflect.Type, data any) (any, error) {
	if from.Kind() != reflect.String || to != reflect.TypeFor[slog.Level]() {
		return data, nil
	}
	l, err := ParseLogLevel(data.(string))
	if err != nil {
		return data, err
	}
	return l, nil
}

// logFormatDecodeHook валидирует формат логирования (text/json), приходящий
// строкой из файла, переменных окружения или флагов.
func logFormatDecodeHook(from reflect.Type, to reflect.Type, data any) (any, error) {
	if from.Kind() != reflect.String || to != reflect.TypeFor[LogFormat]() {
		return data, nil
	}
	f, err := ParseLogFormat(data.(string))
	if err != nil {
		return data, err
	}
	return f, nil
}

// stringToBoolHook учит mapstructure преобразовывать строки "true"/"false" в bool.
func stringToBoolHook(from reflect.Type, to reflect.Type, data any) (any, error) {
	if from.Kind() != reflect.String || to.Kind() != reflect.Bool {
		return data, nil
	}
	switch strings.ToLower(data.(string)) {
	case "true", "1", "yes", "y":
		return true, nil
	case "false", "0", "no", "n", "":
		return false, nil
	}
	return data, nil
}

// loadInto читает конфиг из файла, переменных окружения и CLI-флагов (в порядке возрастания приоритета)
// и распаковывает результат в out.
func loadInto(configPath string, flags *pflag.FlagSet, out any) error {
	k := koanf.New(".")

	// load from config file
	f := file.Provider(configPath)
	if err := k.Load(f, yaml.Parser()); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("load config from file %s: %w", configPath, err)
		}
	}

	// load from CLI flags
	flagLookup := koanf.New(".")
	for _, key := range k.Keys() {
		flagKey := strings.ReplaceAll(key, "_", "-")
		if err := flagLookup.Set(flagKey, k.Get(key)); err != nil {
			return fmt.Errorf("prepare config key %s for flags: %w", flagKey, err)
		}
	}
	if err := loadNormalizedKeys(k, posflag.Provider(flags, ".", flagLookup), nil); err != nil {
		return fmt.Errorf("load config from flags: %w", err)
	}

	// load from env vars
	if err := loadNormalizedKeys(k, env.Provider(".", env.Opt{
		TransformFunc: func(k, v string) (string, any) {
			// Transform the key: SERVER__ADDRESS -> server.address
			k = strings.ReplaceAll(strings.ToLower(k), "__", ".")
			return k, v
		},
	}), nil); err != nil {
		return fmt.Errorf("load config from env: %w", err)
	}

	if err := k.UnmarshalWithConf("", out, koanf.UnmarshalConf{
		DecoderConfig: &mapstructure.DecoderConfig{
			DecodeHook: mapstructure.ComposeDecodeHookFunc(
				durationDecodeHook,
				slogLevelDecodeHook,
				logFormatDecodeHook,
				mapstructure.StringToTimeDurationHookFunc(),
				stringToBoolHook,
				mapstructure.StringToUint16HookFunc(),
			),
		},
	}); err != nil {
		return fmt.Errorf("unmarshall error: %w", err)
	}
	return nil
}

// loadNormalizedKeys загружает источник через koanf, заменяя дефисы в именах
// ключей на подчёркивания для совместимости с CLI-флагами и env-переменными.
func loadNormalizedKeys(k *koanf.Koanf, provider koanf.Provider, parser koanf.Parser) error {
	source := koanf.New(".")
	if err := source.Load(provider, parser); err != nil {
		return err
	}
	for _, key := range source.Keys() {
		normalizedKey := strings.ReplaceAll(key, "-", "_")
		if err := k.Set(normalizedKey, source.Get(key)); err != nil {
			return fmt.Errorf("set config key %s: %w", normalizedKey, err)
		}
	}
	return nil
}

// LoadServerConfig загружает конфигурацию сервера из файла, флагов и переменных
// окружения (приоритет по возрастанию: файл < флаги < окружение).
func LoadServerConfig(configPath string, flags *pflag.FlagSet) (*ServerConfig, error) {
	var cfg ServerConfig
	if err := loadInto(configPath, flags, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// LoadAgentConfig загружает конфигурацию агента и валидирует, что poll-interval,
// report-interval и rate-limit строго положительны.
func LoadAgentConfig(configPath string, flags *pflag.FlagSet) (*AgentConfig, error) {
	var cfg AgentConfig
	if err := loadInto(configPath, flags, &cfg); err != nil {
		return nil, err
	}
	if cfg.PollInterval <= 0 {
		return nil, fmt.Errorf("poll-interval must be strictly positive, got %s", cfg.PollInterval)
	}
	if cfg.ReportInterval <= 0 {
		return nil, fmt.Errorf("report-interval must be strictly positive, got %s", cfg.ReportInterval)
	}
	if cfg.RateLimit <= 0 {
		return nil, fmt.Errorf("rate-limit must be strictly positive, got %d", cfg.RateLimit)
	}
	return &cfg, nil
}
