package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/koanf/v2"
	"github.com/spf13/pflag"
)

const envVarPrefix = "MM_"

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
	Address string    `koanf:"address"`
	Log     LogConfig `koanf:"log"`
}

// конфигурация агента
type AgentConfig struct {
	ServerAddress  string        `koanf:"server_address"`
	PollInterval   time.Duration `koanf:"poll_interval"`
	ReportInterval time.Duration `koanf:"report_interval"`
	Workers        uint16        `koanf:"workers"`
	Log            LogConfig     `koanf:"log"`
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

	k.Load(env.Provider(".", env.Opt{
		Prefix: envVarPrefix,
		TransformFunc: func(k, v string) (string, any) {
			// Transform the key: MM_SERVER_ADDRESS -> server.address
			k = strings.ReplaceAll(strings.ToLower(
				strings.TrimPrefix(k, envVarPrefix)), "_", ".")
			return k, v
		},
	}), nil)

	// load from CLI flags
	if err := k.Load(posflag.Provider(flags, ".", k), nil); err != nil {
		return fmt.Errorf("load config from flags: %w", err)
	}

	if err := k.Unmarshal("", out); err != nil {
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
