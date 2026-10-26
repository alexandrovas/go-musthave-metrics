package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/alexandrovas/go-musthave-metrics/internal/audit"
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
			logger, err := cmdHelper.NewLogger(cfg.Log.Level, string(cfg.Log.Format))
			if err != nil {
				return fmt.Errorf("setup logger: %w", err)
			}
			s := server.New(cfg, logger)
			if err := s.Run(); err != nil {
				return err
			}
			return nil
		},
	}

	cmd.PersistentFlags().StringVarP(&configFile, "config", "c", "config.yaml", "config file path")
	cmd.PersistentFlags().StringP("address", "a", "localhost:8080", "server listen address")
	cmd.PersistentFlags().VarP(cmdHelper.NewDurationValue(300*time.Second), "store-interval", "i", "storage save interval (0 - synchronously write)")
	cmd.PersistentFlags().StringP("file-storage-path", "f", "", "state file storage path")
	cmd.PersistentFlags().BoolP("restore", "r", false, "restore previosly saved state")
	cmd.PersistentFlags().StringP("database-dsn", "d", "", "postgres database dsn")
	cmd.PersistentFlags().StringP("key", "k", "", "key for request/response signing")
	cmd.PersistentFlags().StringP("log.level", "", "info", "log level (debug, info, warn, error)")
	cmd.PersistentFlags().StringP("log.format", "", "text", "log format (text, json)")
	cmd.PersistentFlags().StringP("audit-file", "", "", "path to audit log file (audit disabled if empty)")
	cmd.PersistentFlags().StringP("audit-url", "", "", "URL to send audit events to (audit disabled if empty)")
	cmd.PersistentFlags().IntP("audit-workers", "", audit.DefaultWorkers, "number of concurrent audit delivery workers")
	cmd.PersistentFlags().IntP("audit-queue-size", "", audit.DefaultQueueSize, "audit event queue capacity (events dropped when full)")

	return cmd
}

func main() {
	cmd := cmd()

	if err := cmd.Execute(); err != nil {
		slog.Error(err.Error())
		os.Exit(2)
	}
}
