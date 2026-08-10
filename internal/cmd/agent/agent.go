package agent

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/alexandrovas/go-musthave-metrics/internal/config"
	"github.com/alexandrovas/go-musthave-metrics/internal/models"
	"github.com/alexandrovas/go-musthave-metrics/internal/retry"
)

type Agent struct {
	cfg            *config.AgentConfig
	collector      *collector
	httpClient     *http.Client
	jobs           chan []pendingMetric
	logger         *slog.Logger
	retryIntervals []time.Duration
}

func New(cfg *config.AgentConfig, logger *slog.Logger) *Agent {
	return &Agent{
		cfg: cfg,
		collector: &collector{
			counters: make(map[string]int64),
			gauges:   make(map[string]float64),
		},
		httpClient: &http.Client{
			Timeout: time.Second * 5,
		},
		jobs:           make(chan []pendingMetric, cfg.Workers*10),
		logger:         logger,
		retryIntervals: retry.Intervals,
	}
}

func (a *Agent) Run() error {
	ctx := context.Background()

	a.logger.Info("Agent is running", "server", a.cfg.ServerAddress, "workers", a.cfg.Workers)

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	var wg sync.WaitGroup

	// run multiple workers to push metrics
	a.logger.Info("Start push workers", "count", a.cfg.Workers)
	for idx := range a.cfg.Workers {
		wg.Go(func() {
			a.runWorker(ctx, idx)
		})
	}

	// run report metrics goroutine
	wg.Go(func() {
		a.runReporting(ctx, a.cfg.BatchMode)
	})

	// run poll metrics goroutine
	wg.Go(func() {
		a.runPolling(ctx)
	})

	wg.Wait()
	a.logger.Info("Agent stopped")
	return nil
}

func (a *Agent) runReporting(ctx context.Context, batchMode bool) {
	timer := time.NewTicker(a.cfg.ReportInterval)
	a.logger.Debug("Reporting process started...", "interval", a.cfg.ReportInterval)
	for {
		select {
		case <-timer.C:
			a.report(ctx, batchMode)
			a.logger.Debug("Metrics successfully published")
		case <-ctx.Done():
			timer.Stop()
			a.logger.Debug("Reporting stopped")
			return
		}
	}
}

func (a *Agent) runPolling(ctx context.Context) {
	timer := time.NewTicker(a.cfg.PollInterval)
	a.logger.Debug("Polling process started...", "interval", a.cfg.PollInterval)
	for {
		select {
		case <-timer.C:
			a.collector.poll()
			a.logger.Debug("Metrics successfully polled")
		case <-ctx.Done():
			timer.Stop()
			a.logger.Debug("Polling stopped")
			return
		}
	}
}

func (a *Agent) runWorker(ctx context.Context, idx uint16) {
	log := a.logger.With("worker", idx)
	for {
		select {
		case pms, ok := <-a.jobs:
			if !ok || len(pms) == 0 {
				return
			}
			if len(pms) == 1 {
				if err := a.sendMetric(ctx, pms[0].Metric); err != nil {
					log.Error("send metric", "type", pms[0].Metric.MType, "name", pms[0].Metric.ID, "error", err)
					// restore metric state
					pms[0].Restore()
				}
			} else {
				// in batch mode send all metrics all at once
				metrics := make([]models.Metrics, len(pms))
				for i, m := range pms {
					metrics[i] = m.Metric
				}
				if err := a.sendMetricsBatch(ctx, metrics); err != nil {
					log.Error("send metrics batch", "error", err)
					// restore metrics state
					for _, pm := range pms {
						pm.Restore()
					}
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func (a *Agent) report(ctx context.Context, batchMode bool) {
	pendingMetrics := a.collector.collect()

	if len(pendingMetrics) > 0 {
		if batchMode {
			select {
			// in batch mode report all metrics all at once
			case a.jobs <- pendingMetrics:
			case <-ctx.Done():
				// restore all metrics state
				for _, p := range pendingMetrics {
					p.Restore()
				}
				return
			}
		} else {
			for _, p := range pendingMetrics {
				select {
				case a.jobs <- []pendingMetric{p}:
				case <-ctx.Done():
					p.Restore()
					return
				}
			}
		}
	}
}

func (a *Agent) sendMetricsBatch(ctx context.Context, metrics []models.Metrics) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(metrics); err != nil {
		return fmt.Errorf("json encode error: %w", err)
	}
	url := fmt.Sprintf("http://%s/updates", a.cfg.ServerAddress)
	return a.sendData(ctx, url, buf.Bytes())
}

func (a *Agent) sendMetric(ctx context.Context, metric models.Metrics) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(metric); err != nil {
		return fmt.Errorf("json encode error: %w", err)
	}
	url := fmt.Sprintf("http://%s/update", a.cfg.ServerAddress)
	return a.sendData(ctx, url, buf.Bytes())
}

func (a *Agent) sendData(ctx context.Context, url string, data []byte) error {
	var compressed bytes.Buffer
	gw := gzip.NewWriter(&compressed)
	if _, err := gw.Write(data); err != nil {
		return fmt.Errorf("gzip write error: %w", err)
	}
	if err := gw.Close(); err != nil {
		return fmt.Errorf("gzip close error: %w", err)
	}
	body := compressed.Bytes()

	return retry.Do(ctx, isRetriableSendError, a.retryIntervals, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Content-Encoding", "gzip")
		req.Header.Set("Accept-Encoding", "gzip")

		resp, err := a.httpClient.Do(req)
		if err != nil {
			return err
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status %s", resp.Status)
		}
		return nil
	})
}

// isRetriableSendError сообщает, стоит ли повторить отправку: временные сетевые
// ошибки (отказ в соединении, таймаут, DNS) считаются повторяемыми, ответ сервера
// с неуспешным статусом — нет, повтор не решит проблему валидации/данных.
func isRetriableSendError(err error) bool {
	_, ok := errors.AsType[net.Error](err)
	return ok
}
