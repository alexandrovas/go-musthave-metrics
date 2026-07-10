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

var (
	rootCmd = &cobra.Command{
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
