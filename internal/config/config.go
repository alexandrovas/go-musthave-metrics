package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

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

type Config struct {
	Server ServerConfig `koanf:"server"`
	Log    LogConfig    `koanf:"log"`
}

type ServerConfig struct {
	Address string `koanf:"address"`
}

type LogConfig struct {
	Level  string    `koanf:"level"`
	Format LogFormat `koanf:"format"`
}

func Load(configPath string, flags *pflag.FlagSet) (*Config, error) {
	k := koanf.New(".")

	// load from config file
	f := file.Provider(configPath)
	if err := k.Load(f, yaml.Parser()); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("load config from file %s: %w", configPath, err)
		}
	}

	// load from env vars
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
		return nil, fmt.Errorf("load config from flags: %w", err)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("unmarshall error: %w", err)
	}
	return &cfg, nil
}
