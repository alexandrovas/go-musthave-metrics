package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/alexandrovas/go-musthave-metrics/internal/cmd/server"
	"github.com/alexandrovas/go-musthave-metrics/internal/config"
)

func setupLogger(level string, format string) error {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil {
		l = slog.LevelInfo
	}
	var h slog.Handler
	switch config.LogFormat(format) {
	case config.TextLogFormat:
		h = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: l,
		})
	case config.JsonLogFormat:
		h = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: l,
		})
	default:
		return fmt.Errorf("unexpected log format: %s", format)
	}
	slog.SetDefault(slog.New(h))
	return nil
}

var (
	rootCmd = &cobra.Command{
		Use:   "musthave-metrics-server",
		Short: "musthave-metrics is metrics storage",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadServerConfig(configFile, cmd.Flags())
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			if err := setupLogger(cfg.Log.Level, string(cfg.Log.Format)); err != nil {
				return fmt.Errorf("setup logger: %w", err)
			}
			if err := server.Run(cfg); err != nil {
				return err
			}
			return nil
		},
	}
	configFile string
)

func flags() {
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "config.yaml", "config file path")
	rootCmd.PersistentFlags().StringP("address", "a", "localhost:8080", "server listen address")
	rootCmd.PersistentFlags().StringP("log.level", "", "info", "log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().StringP("log.format", "", "text", "log format (text, json)")
}

func main() {
	flags()

	if err := rootCmd.Execute(); err != nil {
		slog.Error(err.Error())
		os.Exit(2)
	}
}
