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
		Use:         "musthave-metrics-server",
		Short:       "musthave-metrics is metrics storage",
		Version:     cmdHelper.BuildVersion(),
		Annotations: cmdHelper.CobraAnnotations(),

		// Отключаем стандартный вывод usage при ошибках, используем свой, который подключается с
		// помощью SetFlagErrorFunc ниже.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Выводим build-информацию до загрузки конфигурации и запуска сервера.
			cmdHelper.PrintBuildInfo(os.Stdout)
			cfg, err := config.LoadServerConfig(configFile, cmd.Flags())
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			logger := cmdHelper.NewLogger(cfg.Log.Level, cfg.Log.Format)
			s := server.New(cfg, logger)
			if err := s.Run(); err != nil {
				logger.Error("server process terminated abnormally", "error", err)
				return fmt.Errorf("server process terminated abnormally: %w", err)
			}
			return nil
		},
	}
	cmd.SetVersionTemplate(cmdHelper.CobraVersionTemplate)

	// usage показываем только при ошибке разбора флагов: cobra вызывает
	// FlagErrorFunc исключительно для ошибок парсинга флагов, поэтому при ошибке
	// RunE usage не выводится.
	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		c.Println(c.UsageString())
		return err
	})

	cmd.PersistentFlags().StringVarP(&configFile, "config", "c", "config.yaml", "config file path")
	cmd.PersistentFlags().StringP("address", "a", "localhost:8080", "server listen address")
	cmd.PersistentFlags().VarP(cmdHelper.NewDurationValue(300*time.Second), "store-interval", "i", "storage save interval (0 - synchronously write)")
	cmd.PersistentFlags().StringP("file-storage-path", "f", "", "state file storage path")
	cmd.PersistentFlags().BoolP("restore", "r", false, "restore previosly saved state")
	cmd.PersistentFlags().StringP("database-dsn", "d", "", "postgres database dsn")
	cmd.PersistentFlags().StringP("key", "k", "", "key for request/response signing")
	cmd.PersistentFlags().VarP(cmdHelper.NewLogLevelValue(slog.LevelInfo), "log.level", "", "log level (debug, info, warn, error)")
	cmd.PersistentFlags().VarP(cmdHelper.NewLogFormatValue(config.TextLogFormat), "log.format", "", "log format (text, json)")
	cmd.PersistentFlags().StringP("audit-file", "", "", "path to audit log file (audit disabled if empty)")
	cmd.PersistentFlags().StringP("audit-url", "", "", "URL to send audit events to (audit disabled if empty)")
	cmd.PersistentFlags().IntP("audit-workers", "", audit.DefaultWorkers, "number of concurrent audit delivery workers")
	cmd.PersistentFlags().IntP("audit-queue-size", "", audit.DefaultQueueSize, "audit event queue capacity (events dropped when full)")

	return cmd
}

// Кастомный статический анализатор staticlint запрещает использовать os.Exit()
// в функции main(), поэтому используем дополнительный хелпер.
func execute() {
	if err := cmd().Execute(); err != nil {
		// Ошибку тут не логируем, потому что её выводит cobra в stderr
		os.Exit(1)
	}
}

func main() {
	execute()
}
