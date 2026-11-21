package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func missingConfigPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "missing.yaml")
}

func writeConfigFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	oldValue, existed := os.LookupEnv(key)
	require.NoError(t, os.Unsetenv(key))
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(key, oldValue)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

func newServerFlagSet() *pflag.FlagSet {
	fs := pflag.NewFlagSet("server", pflag.ContinueOnError)
	fs.StringP("address", "a", "localhost:8080", "")
	fs.DurationP("store-interval", "i", 300*time.Second, "")
	fs.StringP("file-storage-path", "f", "", "")
	fs.BoolP("restore", "r", false, "")
	fs.StringP("database-dsn", "d", "", "")
	fs.StringP("key", "k", "", "")
	fs.String("crypto-key", "", "")
	fs.StringP("audit-file", "", "", "")
	fs.StringP("audit-url", "", "", "")
	fs.IntP("audit-workers", "", 4, "")
	fs.IntP("audit-queue-size", "", 64, "")
	fs.StringP("log.level", "", "info", "")
	fs.StringP("log.format", "", "text", "")
	return fs
}

func newAgentFlagSet() *pflag.FlagSet {
	fs := pflag.NewFlagSet("agent", pflag.ContinueOnError)
	fs.StringP("address", "a", "localhost:8080", "")
	fs.DurationP("poll-interval", "p", 2*time.Second, "")
	fs.DurationP("report-interval", "r", 10*time.Second, "")
	fs.Uint16P("rate-limit", "l", 5, "")
	fs.StringP("log.level", "", "info", "")
	fs.StringP("log.format", "", "text", "")
	fs.StringP("key", "k", "", "")
	fs.String("crypto-key", "", "")
	return fs
}

func TestDurationDecodeHook(t *testing.T) {
	durType := reflect.TypeFor[time.Duration]()
	strType := reflect.TypeFor[string]()

	tests := []struct {
		name    string
		from    reflect.Type
		to      reflect.Type
		data    any
		want    any
		wantErr bool
	}{
		{
			name: "bare number is seconds",
			from: strType,
			to:   durType,
			data: "30",
			want: 30 * time.Second,
		},
		{
			name: "standard duration",
			from: strType,
			to:   durType,
			data: "1m",
			want: time.Minute,
		},
		{
			name:    "invalid string returns error",
			from:    strType,
			to:      durType,
			data:    "abc",
			want:    "abc",
			wantErr: true,
		},
		{
			name: "non-string source passes through",
			from: reflect.TypeFor[int](),
			to:   durType,
			data: 5,
			want: 5,
		},
		{
			name: "non-duration target passes through",
			from: strType,
			to:   strType,
			data: "30",
			want: "30",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := durationDecodeHook(tt.from, tt.to, tt.data)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestStringToBoolHook(t *testing.T) {
	boolType := reflect.TypeFor[bool]()
	strType := reflect.TypeFor[string]()

	tests := []struct {
		name string
		from reflect.Type
		to   reflect.Type
		data any
		want any
	}{
		{"true", strType, boolType, "true", true},
		{"1", strType, boolType, "1", true},
		{"yes", strType, boolType, "yes", true},
		{"y", strType, boolType, "y", true},
		{"uppercase true", strType, boolType, "TRUE", true},
		{"false", strType, boolType, "false", false},
		{"0", strType, boolType, "0", false},
		{"no", strType, boolType, "no", false},
		{"n", strType, boolType, "n", false},
		{"empty", strType, boolType, "", false},
		{"unknown value passes through", strType, boolType, "maybe", "maybe"},
		{"non-string source passes through", reflect.TypeFor[int](), boolType, 1, 1},
		{"non-bool target passes through", strType, strType, "true", "true"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := stringToBoolHook(tt.from, tt.to, tt.data)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLoadServerConfig_FromFlags(t *testing.T) {
	fs := newServerFlagSet()
	require.NoError(t, fs.Set("address", "flag-host:9090"))
	require.NoError(t, fs.Set("store-interval", "45s"))
	require.NoError(t, fs.Set("file-storage-path", "/tmp/flag-metrics.json"))
	require.NoError(t, fs.Set("restore", "true"))
	require.NoError(t, fs.Set("database-dsn", "postgres://flag"))
	require.NoError(t, fs.Set("key", "flag-key"))
	require.NoError(t, fs.Set("crypto-key", "/tmp/flag-public.pem"))
	require.NoError(t, fs.Set("log.level", "warn"))
	require.NoError(t, fs.Set("log.format", "json"))
	require.NoError(t, fs.Set("audit-workers", "8"))
	require.NoError(t, fs.Set("audit-queue-size", "128"))

	cfg, err := LoadServerConfig(missingConfigPath(t), fs)
	require.NoError(t, err)

	assert.Equal(t, "flag-host:9090", cfg.Address)
	assert.Equal(t, 45*time.Second, cfg.StoreInterval)
	assert.Equal(t, "/tmp/flag-metrics.json", cfg.FileStoragePath)
	assert.True(t, cfg.RestoreState)
	assert.Equal(t, "postgres://flag", cfg.DatabaseDSN)
	assert.Equal(t, "flag-key", cfg.Key)
	assert.Equal(t, "/tmp/flag-public.pem", cfg.CryptoKey)
	assert.Equal(t, slog.LevelWarn, cfg.Log.Level)
	assert.Equal(t, JSONLogFormat, cfg.Log.Format)
	assert.Equal(t, 8, cfg.AuditWorkers)
	assert.Equal(t, 128, cfg.AuditQueueSize)
}

func TestLoadServerConfig_FromYAMLFile(t *testing.T) {
	path := writeConfigFile(t, `
address: "file-host:8080"
store_interval: "30"
file_storage_path: "/tmp/file-metrics.json"
restore: "true"
database_dsn: "postgres://file"
key: "file-key"
crypto_key: "/tmp/file-private.pem"
audit_file: "/tmp/audit.log"
audit_url: "http://audit.example.com"
audit_workers: 2
audit_queue_size: 16
log:
  level: "debug"
  format: "json"
`)

	cfg, err := LoadServerConfig(path, pflag.NewFlagSet("empty", pflag.ContinueOnError))
	require.NoError(t, err)

	assert.Equal(t, "file-host:8080", cfg.Address)
	assert.Equal(t, 30*time.Second, cfg.StoreInterval)
	assert.Equal(t, "/tmp/file-metrics.json", cfg.FileStoragePath)
	assert.True(t, cfg.RestoreState)
	assert.Equal(t, "postgres://file", cfg.DatabaseDSN)
	assert.Equal(t, "file-key", cfg.Key)
	assert.Equal(t, "/tmp/file-private.pem", cfg.CryptoKey)
	assert.Equal(t, "/tmp/audit.log", cfg.AuditFile)
	assert.Equal(t, "http://audit.example.com", cfg.AuditURL)
	assert.Equal(t, 2, cfg.AuditWorkers)
	assert.Equal(t, 16, cfg.AuditQueueSize)
	assert.Equal(t, slog.LevelDebug, cfg.Log.Level)
	assert.Equal(t, JSONLogFormat, cfg.Log.Format)
}

func TestLoadServerConfig_FileValueIsNotOverriddenByUnsetFlag(t *testing.T) {
	path := writeConfigFile(t, `store_interval: "45s"`)
	flags := newServerFlagSet()

	cfg, err := LoadServerConfig(path, flags)
	require.NoError(t, err)
	assert.Equal(t, 45*time.Second, cfg.StoreInterval)

	require.NoError(t, flags.Set("store-interval", "90s"))
	cfg, err = LoadServerConfig(path, flags)
	require.NoError(t, err)
	assert.Equal(t, 90*time.Second, cfg.StoreInterval)
}

func TestLoadServerConfig_FileValueIsNotOverriddenByUnsetEnv(t *testing.T) {
	unsetEnv(t, "STORE_INTERVAL")
	path := writeConfigFile(t, `store_interval: "45s"`)

	cfg, err := LoadServerConfig(path, newServerFlagSet())
	require.NoError(t, err)
	assert.Equal(t, 45*time.Second, cfg.StoreInterval)
}

func TestLoadServerConfig_EnvOverridesFileAndFlag(t *testing.T) {
	path := writeConfigFile(t, `store_interval: "45s"`)
	flags := newServerFlagSet()
	require.NoError(t, flags.Set("store-interval", "60s"))
	t.Setenv("STORE_INTERVAL", "90s")

	cfg, err := LoadServerConfig(path, flags)
	require.NoError(t, err)
	assert.Equal(t, 90*time.Second, cfg.StoreInterval)
}

func TestLoadServerConfig_FromJSONFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	content := `{
		"address": "localhost:8080",
		"restore": true,
		"store_interval": "1s",
		"file_storage_path": "/path/to/file.db",
		"database_dsn": "",
		"crypto_key": "/path/to/key.pem"
	}`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	cfg, err := LoadServerConfig(path, newServerFlagSet())
	require.NoError(t, err)
	assert.Equal(t, "localhost:8080", cfg.Address)
	assert.True(t, cfg.RestoreState)
	assert.Equal(t, 1*time.Second, cfg.StoreInterval)
	assert.Equal(t, "/path/to/file.db", cfg.FileStoragePath)
	assert.Equal(t, "", cfg.DatabaseDSN)
	assert.Equal(t, "/path/to/key.pem", cfg.CryptoKey)
}

func TestLoadServerConfig_EnvOverridesFlag(t *testing.T) {
	t.Setenv("ADDRESS", "env-host:8080")
	t.Setenv("CRYPTO_KEY", "/tmp/env-private.pem")

	fs := newServerFlagSet()
	require.NoError(t, fs.Set("address", "flag-host:9090"))

	cfg, err := LoadServerConfig(missingConfigPath(t), fs)
	require.NoError(t, err)
	assert.Equal(t, "env-host:8080", cfg.Address)
	assert.Equal(t, "/tmp/env-private.pem", cfg.CryptoKey)
}

func TestLoadAgentConfig_CryptoKeyFromEnv(t *testing.T) {
	t.Setenv("CRYPTO_KEY", "/tmp/env-public.pem")

	cfg, err := LoadAgentConfig(missingConfigPath(t), newAgentFlagSet())
	require.NoError(t, err)
	assert.Equal(t, "/tmp/env-public.pem", cfg.CryptoKey)
}

func TestLoadServerConfig_EnvBareNumberDuration(t *testing.T) {
	t.Setenv("STORE_INTERVAL", "30")

	cfg, err := LoadServerConfig(missingConfigPath(t), pflag.NewFlagSet("empty", pflag.ContinueOnError))
	require.NoError(t, err)
	assert.Equal(t, 30*time.Second, cfg.StoreInterval)
}

func TestLoadAgentConfig_FromFlags(t *testing.T) {
	fs := newAgentFlagSet()
	require.NoError(t, fs.Set("address", "flag-host:9090"))
	require.NoError(t, fs.Set("poll-interval", "3s"))
	require.NoError(t, fs.Set("report-interval", "15s"))
	require.NoError(t, fs.Set("rate-limit", "7"))
	require.NoError(t, fs.Set("key", "agent-key"))
	require.NoError(t, fs.Set("crypto-key", "/tmp/agent-public.pem"))
	require.NoError(t, fs.Set("log.level", "error"))
	require.NoError(t, fs.Set("log.format", "text"))

	cfg, err := LoadAgentConfig(missingConfigPath(t), fs)
	require.NoError(t, err)

	assert.Equal(t, "flag-host:9090", cfg.ServerAddress)
	assert.Equal(t, 3*time.Second, cfg.PollInterval)
	assert.Equal(t, 15*time.Second, cfg.ReportInterval)
	assert.Equal(t, uint16(7), cfg.RateLimit)
	assert.Equal(t, "agent-key", cfg.Key)
	assert.Equal(t, "/tmp/agent-public.pem", cfg.CryptoKey)
	assert.Equal(t, slog.LevelError, cfg.Log.Level)
	assert.Equal(t, TextLogFormat, cfg.Log.Format)
}

func TestLoadAgentConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(fs *pflag.FlagSet)
		wantErr string
	}{
		{
			name:    "poll-interval zero",
			mutate:  func(fs *pflag.FlagSet) { _ = fs.Set("poll-interval", "0s") },
			wantErr: "poll-interval must be strictly positive",
		},
		{
			name:    "poll-interval negative",
			mutate:  func(fs *pflag.FlagSet) { _ = fs.Set("poll-interval", "-1s") },
			wantErr: "poll-interval must be strictly positive",
		},
		{
			name:    "report-interval zero",
			mutate:  func(fs *pflag.FlagSet) { _ = fs.Set("report-interval", "0s") },
			wantErr: "report-interval must be strictly positive",
		},
		{
			name:    "rate-limit zero",
			mutate:  func(fs *pflag.FlagSet) { _ = fs.Set("rate-limit", "0") },
			wantErr: "rate-limit must be strictly positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := newAgentFlagSet()
			tt.mutate(fs)

			_, err := LoadAgentConfig(missingConfigPath(t), fs)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestLogFormatDecodeHook(t *testing.T) {
	formatType := reflect.TypeFor[LogFormat]()
	strType := reflect.TypeFor[string]()

	tests := []struct {
		name    string
		from    reflect.Type
		to      reflect.Type
		data    any
		want    any
		wantErr bool
	}{
		{"text", strType, formatType, "text", TextLogFormat, false},
		{"json", strType, formatType, "json", JSONLogFormat, false},
		{"invalid returns error", strType, formatType, "yaml", "yaml", true},
		{"non-string source passes through", reflect.TypeFor[int](), formatType, 5, 5, false},
		{"non-format target passes through", strType, strType, "text", "text", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := logFormatDecodeHook(tt.from, tt.to, tt.data)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLoadServerConfig_InvalidLogFormat(t *testing.T) {
	path := writeConfigFile(t, "log:\n  level: \"info\"\n  format: \"yaml\"\n")

	_, err := LoadServerConfig(path, pflag.NewFlagSet("empty", pflag.ContinueOnError))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid log format")
}

func TestLoadServerConfig_InvalidLogLevel(t *testing.T) {
	path := writeConfigFile(t, "log:\n  level: \"bogus\"\n  format: \"text\"\n")

	_, err := LoadServerConfig(path, pflag.NewFlagSet("empty", pflag.ContinueOnError))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid log level")
}
