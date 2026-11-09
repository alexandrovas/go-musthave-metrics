package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/alexandrovas/go-musthave-metrics/internal/cmd/agent"
	cmdHelper "github.com/alexandrovas/go-musthave-metrics/internal/cmd/helper"
	"github.com/alexandrovas/go-musthave-metrics/internal/config"
)

func cmd() *cobra.Command {
	var configFile string

	cmd := &cobra.Command{
		Use:         "musthave-metrics-agent",
		Short:       "musthave-metrics is metrics storage",
		Version:     cmdHelper.BuildVersion(),
		Annotations: cmdHelper.CobraAnnotations(),

		// Отключаем стандартный вывод usage при ошибках, используем свой, который подключается с
		// помощью SetFlagErrorFunc ниже.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Выводим build-информацию до загрузки конфигурации и запуска агента.
			cmdHelper.PrintBuildInfo(cmd.OutOrStderr())
			cfg, err := config.LoadAgentConfig(configFile, cmd.Flags())
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			logger := cmdHelper.NewLogger(cfg.Log.Level, cfg.Log.Format)
			a := agent.New(cfg, logger)
			if err := a.Run(); err != nil {
				logger.Error("agent process terminated abnormally", "error", err)
				return fmt.Errorf("agent process terminated abnormally: %w", err)
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
	cmd.PersistentFlags().StringP("address", "a", "localhost:8080", "server address")
	cmd.PersistentFlags().VarP(cmdHelper.NewDurationValue(2*time.Second), "poll-interval", "p", "Metrics poll interval (e.g. 2s or 2)")
	cmd.PersistentFlags().VarP(cmdHelper.NewDurationValue(10*time.Second), "report-interval", "r", "Metrics report interval (e.g. 10s or 10)")
	cmd.PersistentFlags().Uint16P("rate-limit", "l", 5, "max number of simultaneously outgoing requests to the server")
	cmd.PersistentFlags().VarP(cmdHelper.NewLogLevelValue(slog.LevelInfo), "log.level", "", "log level (debug, info, warn, error)")
	cmd.PersistentFlags().VarP(cmdHelper.NewLogFormatValue(config.TextLogFormat), "log.format", "", "log format (text, json)")
	cmd.PersistentFlags().StringP("key", "k", "", "key for request signing")

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
