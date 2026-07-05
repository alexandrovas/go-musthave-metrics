package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/alexandrovas/go-musthave-metrics/internal/cmd/agent"
	"github.com/alexandrovas/go-musthave-metrics/internal/config"
	"github.com/spf13/cobra"
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
		Use:   "musthave-metrics-agent",
		Short: "musthave-metrics is metrics storage",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadAgentConfig(configFile, cmd.Flags())
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			if err := setupLogger(cfg.Log.Level, string(cfg.Log.Format)); err != nil {
				return fmt.Errorf("setup logger: %w", err)
			}
			if err := agent.Run(cfg); err != nil {
				return err
			}
			return nil
		},
	}
	configFile string
)

func flags() {
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "config.yaml", "config file path")
	rootCmd.PersistentFlags().StringP("server_address", "a", "localhost:8080", "server address")
	rootCmd.PersistentFlags().VarP(newDurationValue(2*time.Second), "poll_interval", "p", "Metrics poll interval (e.g. 2s or 2)")
	rootCmd.PersistentFlags().VarP(newDurationValue(10*time.Second), "report_interval", "r", "Metrics report interval (e.g. 10s or 10)")
	rootCmd.PersistentFlags().Uint16P("workers", "w", 5, "Workers count")
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
