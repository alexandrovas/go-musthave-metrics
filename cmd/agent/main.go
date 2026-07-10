package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/alexandrovas/go-musthave-metrics/internal/cmd/agent"
	cmdHelper "github.com/alexandrovas/go-musthave-metrics/internal/cmd/helper"
	"github.com/alexandrovas/go-musthave-metrics/internal/config"
	"github.com/spf13/cobra"
)

var (
	rootCmd = &cobra.Command{
		Use:   "musthave-metrics-agent",
		Short: "musthave-metrics is metrics storage",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadAgentConfig(configFile, cmd.Flags())
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			if err := cmdHelper.SetupLogger(cfg.Log.Level, string(cfg.Log.Format)); err != nil {
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
