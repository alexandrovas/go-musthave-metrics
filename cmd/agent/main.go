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

func cmd() *cobra.Command {
	var configFile string

	cmd := &cobra.Command{
		Use:   "musthave-metrics-agent",
		Short: "musthave-metrics is metrics storage",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadAgentConfig(configFile, cmd.Flags())
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			logger, err := cmdHelper.NewLogger(cfg.Log.Level, string(cfg.Log.Format))
			if err != nil {
				return fmt.Errorf("setup logger: %w", err)
			}
			a := agent.New(cfg, logger)
			if err := a.Run(); err != nil {
				return err
			}
			return nil
		},
	}

	cmd.PersistentFlags().StringVarP(&configFile, "config", "c", "config.yaml", "config file path")
	cmd.PersistentFlags().StringP("address", "a", "localhost:8080", "server address")
	cmd.PersistentFlags().VarP(cmdHelper.NewDurationValue(2*time.Second), "poll_interval", "p", "Metrics poll interval (e.g. 2s or 2)")
	cmd.PersistentFlags().VarP(cmdHelper.NewDurationValue(10*time.Second), "report_interval", "r", "Metrics report interval (e.g. 10s or 10)")
	cmd.PersistentFlags().Uint16P("workers", "w", 5, "Workers count")
	cmd.PersistentFlags().StringP("log.level", "", "info", "log level (debug, info, warn, error)")
	cmd.PersistentFlags().StringP("log.format", "", "text", "log format (text, json)")

	return cmd
}

func main() {
	cmd := cmd()

	if err := cmd.Execute(); err != nil {
		slog.Error(err.Error())
		os.Exit(2)
	}
}
