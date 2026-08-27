package agent

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/alexandrovas/go-musthave-metrics/internal/collectors"
	"github.com/alexandrovas/go-musthave-metrics/internal/config"
	"github.com/alexandrovas/go-musthave-metrics/internal/models"
	"github.com/alexandrovas/go-musthave-metrics/internal/retry"
	"github.com/alexandrovas/go-musthave-metrics/internal/sign"
)

// job — единица работы воркера: одна метрика (batch=false) либо весь батч,
// собранный за один цикл report (batch=true). deadline общий для всех job,
// отправленных в рамках одного цикла report — это ограничивает суммарное
// время, которое агент готов потратить на доставку метрик этого цикла
// (включая все ретраи), независимо от того, сколько отдельных job из него
// получилось.
type job struct {
	metrics  []collectors.PendingMetric
	deadline time.Time
}

// collector — общий интерфейс коллектора
type collector interface {
	// Обновляет внутреннее хранилище
	Poll()

	// Cнимает с хранилище снимок накопленных метрик
	Collect() []collectors.PendingMetric
}

type Agent struct {
	cfg            *config.AgentConfig
	collectors     []collector
	httpClient     *http.Client
	jobs           chan job
	logger         *slog.Logger
	retryIntervals []time.Duration
}

func New(cfg *config.AgentConfig, logger *slog.Logger) *Agent {
	return &Agent{
		cfg: cfg,
		collectors: []collector{
			collectors.NewRuntime(logger.With("collector", "runtime")),
			collectors.NewGopsutil(logger.With("collector", "gopsutil")),
		},
		httpClient: &http.Client{
			Timeout: time.Second * 5,
		},
		jobs:           make(chan job, cfg.RateLimit),
		logger:         logger,
		retryIntervals: retry.Intervals,
	}
}

func (a *Agent) Run() error {
	ctx := context.Background()

	a.logger.Info("Agent is running", "server", a.cfg.ServerAddress, "rate_limit", a.cfg.RateLimit)

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	var wg sync.WaitGroup

	// run multiple workers to push metrics; их количество ограничивает число
	// одновременно исходящих запросов на сервер сверху (rate limit).
	a.logger.Info("Start push workers", "count", a.cfg.RateLimit)
	for idx := range a.cfg.RateLimit {
		wg.Go(func() {
			a.runWorker(ctx, idx)
		})
	}

	// run report metrics goroutine
	wg.Go(func() {
		a.runReporting(ctx, a.cfg.BatchMode)
	})

	// run poll metrics goroutine — внутри запускает по горутине на каждый
	// коллектор.
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
		case <-ctx.Done():
			timer.Stop()
			a.logger.Debug("Reporting stopped")
			return
		}
	}
}

// runPolling запускает независимую горутину опроса для каждого коллектора.
func (a *Agent) runPolling(ctx context.Context) {
	var wg sync.WaitGroup
	for _, c := range a.collectors {
		wg.Go(func() {
			col := c

			timer := time.NewTicker(a.cfg.PollInterval)
			defer timer.Stop()

			for {
				select {
				case <-timer.C:
					col.Poll()
				case <-ctx.Done():
					return
				}
			}
		})
	}
	wg.Wait()
	a.logger.Debug("Polling stopped")
}

func (a *Agent) runWorker(ctx context.Context, idx uint16) {
	log := a.logger.With("worker", idx)
	for {
		select {
		case j, ok := <-a.jobs:
			if !ok || len(j.metrics) == 0 {
				return
			}

			sendCtx, cancel := context.WithDeadline(ctx, j.deadline)

			if len(j.metrics) == 1 {
				if err := a.sendMetric(sendCtx, j.metrics[0].Metric); err != nil {
					log.Error("send metric",
						"type", j.metrics[0].Metric.MType,
						"name", j.metrics[0].Metric.ID,
						"error", err)
					j.metrics[0].Restore()
				}
			} else {
				// in batch mode send all metrics all at once
				metrics := make([]models.Metrics, len(j.metrics))
				for i, m := range j.metrics {
					metrics[i] = m.Metric
				}
				if err := a.sendMetricsBatch(sendCtx, metrics); err != nil {
					log.Error("send metrics batch", "error", err)
					for _, pm := range j.metrics {
						pm.Restore()
					}
				}
			}

			cancel()
		case <-ctx.Done():
			return
		}
	}
}

func (a *Agent) report(ctx context.Context, batchMode bool) {
	// снимаем снимки со всех коллекторов параллельно и объединяем результаты
	results := make([][]collectors.PendingMetric, len(a.collectors))
	var wg sync.WaitGroup
	for i, c := range a.collectors {
		wg.Go(func() {
			results[i] = c.Collect()
		})
	}
	wg.Wait()

	// обхединяем метрики в один slice
	var pendingMetrics []collectors.PendingMetric
	for _, r := range results {
		pendingMetrics = append(pendingMetrics, r...)
	}

	// если метрик нет, выходим
	if len(pendingMetrics) == 0 {
		return
	}

	// Используем ReportInterval как deadline
	// на отправку одного полного списка метрик.
	// Если отправка метрик за отведенное время не удалась, то отменяем
	// отправку и начинаем новый цикл.
	deadline := time.Now().Add(a.cfg.ReportInterval)

	if batchMode {
		select {
		// in batch mode report all metrics all at once
		case a.jobs <- job{
			metrics:  pendingMetrics,
			deadline: deadline,
		}:
		case <-ctx.Done():
			// restore all metrics state
			for _, p := range pendingMetrics {
				p.Restore()
			}
		}
	} else {
		for _, p := range pendingMetrics {
			select {
			case a.jobs <- job{
				metrics:  []collectors.PendingMetric{p},
				deadline: deadline,
			}:
			case <-ctx.Done():
				p.Restore()
				return
			}
		}
	}

	a.logger.Debug("Metrics successfully scheduled to send")
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

const hashHeader = "HashSHA256"

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

	// Хеш считается от исходных (несжатых) данных — именно их сервер увидит
	// после декомпрессии тела запроса.
	var hash string
	if a.cfg.Key != "" {
		hash = sign.Compute(data, a.cfg.Key)
	}

	return retry.Do(ctx, isRetriableSendError, a.retryIntervals,
		func() error {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
			if err != nil {
				return err
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Content-Encoding", "gzip")
			req.Header.Set("Accept-Encoding", "gzip")
			if hash != "" {
				req.Header.Set(hashHeader, hash)
			}

			resp, err := a.httpClient.Do(req)
			if err != nil {
				a.logger.Warn("failed to send request", "error", err)
				return err
			}
			defer resp.Body.Close()

			body, err = io.ReadAll(resp.Body)
			if err != nil {
				a.logger.Warn("failed to send request", "error", err)
				return fmt.Errorf("failed to read body: %w", err)
			}

			if resp.StatusCode != http.StatusOK {
				a.logger.Warn("bad status code from server", "error", err, "statusCode", resp.StatusCode, "body", string(body))
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
