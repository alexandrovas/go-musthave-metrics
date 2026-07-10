package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	cmdHelper "github.com/alexandrovas/go-musthave-metrics/internal/cmd/helper"
	"github.com/alexandrovas/go-musthave-metrics/internal/cmd/server"
	"github.com/alexandrovas/go-musthave-metrics/internal/config"
)

func cmd() *cobra.Command {
	var configFile string

	cmd := &cobra.Command{
		Use:   "musthave-metrics-server",
		Short: "musthave-metrics is metrics storage",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadServerConfig(configFile, cmd.Flags())
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			if err := cmdHelper.SetupLogger(cfg.Log.Level, string(cfg.Log.Format)); err != nil {
				return fmt.Errorf("setup logger: %w", err)
			}
			if err := server.Run(cfg); err != nil {
				return err
			}
			return nil
		},
	}

	cmd.PersistentFlags().StringVarP(&configFile, "config", "c", "config.yaml", "config file path")
	cmd.PersistentFlags().StringP("address", "a", "localhost:8080", "server listen address")
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
