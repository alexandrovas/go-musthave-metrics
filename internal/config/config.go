package config

import (
	"errors"
	"fmt"
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

type LogFormat string

const (
	JsonLogFormat LogFormat = "json"
	TextLogFormat LogFormat = "text"
)

type LogConfig struct {
	Level  string    `koanf:"level"`
	Format LogFormat `koanf:"format"`
}

// конфигурация сервера
type ServerConfig struct {
	Address         string        `koanf:"address"`
	Log             LogConfig     `koanf:"log"`
	StoreInterval   time.Duration `koanf:"store_interval"`
	FileStoragePath string        `koanf:"file_storage_path"`
	RestoreState    bool          `koanf:"restore"`
}

// конфигурация агента
type AgentConfig struct {
	ServerAddress  string        `koanf:"address"`
	PollInterval   time.Duration `koanf:"poll_interval"`
	ReportInterval time.Duration `koanf:"report_interval"`
	Workers        uint16        `koanf:"workers"`
	Log            LogConfig     `koanf:"log"`
}

// durationDecodeHook учит mapstructure понимать голые числа как секунды.
func durationDecodeHook(from reflect.Type, to reflect.Type, data any) (any, error) {
	if from.Kind() != reflect.String || to != reflect.TypeOf(time.Duration(0)) {
		return data, nil
	}
	d, err := ParseDuration(data.(string))
	if err != nil {
		return data, err
	}
	return d, nil
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
	if err := k.Load(posflag.Provider(flags, ".", k), nil); err != nil {
		return fmt.Errorf("load config from flags: %w", err)
	}

	// load from env vars
	if err := k.Load(env.Provider(".", env.Opt{
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
				mapstructure.StringToTimeDurationHookFunc(),
				stringToBoolHook,
			),
		},
	}); err != nil {
		return fmt.Errorf("unmarshall error: %w", err)
	}
	return nil
}

func LoadServerConfig(configPath string, flags *pflag.FlagSet) (*ServerConfig, error) {
	var cfg ServerConfig
	if err := loadInto(configPath, flags, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func LoadAgentConfig(configPath string, flags *pflag.FlagSet) (*AgentConfig, error) {
	var cfg AgentConfig
	if err := loadInto(configPath, flags, &cfg); err != nil {
		return nil, err
	}
	if cfg.PollInterval <= 0 {
		return nil, fmt.Errorf("poll_interval must be strictly positive, got %s", cfg.PollInterval)
	}
	if cfg.ReportInterval <= 0 {
		return nil, fmt.Errorf("report_interval must be strictly positive, got %s", cfg.ReportInterval)
	}
	if cfg.Workers <= 0 {
		return nil, fmt.Errorf("workers must be strictly positive, got %d", cfg.Workers)
	}
	return &cfg, nil
}
