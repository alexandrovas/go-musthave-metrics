package agent

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/alexandrovas/go-musthave-metrics/internal/collectors"
	"github.com/alexandrovas/go-musthave-metrics/internal/config"
	"github.com/alexandrovas/go-musthave-metrics/internal/crypto"
	"github.com/alexandrovas/go-musthave-metrics/internal/models"
	"github.com/alexandrovas/go-musthave-metrics/internal/pool"
	"github.com/alexandrovas/go-musthave-metrics/internal/retry"
	"github.com/alexandrovas/go-musthave-metrics/internal/sign"
)

// job — единица работы воркера: пакет метрик, поставленный в очередь на отправку.
type job struct {
	metrics []collectors.PendingMetric
}

// collector — общий интерфейс коллектора
type collector interface {
	// Обновляет внутреннее хранилище
	Poll()

	// Cнимает с хранилища снимок накопленных метрик
	Collect() []collectors.PendingMetric
}

// Компиляционная проверка: коллекторы удовлетворяют интерфейсу collector.
var (
	_ collector = (*collectors.Runtime)(nil)
	_ collector = (*collectors.Gopsutil)(nil)
)

// Agent — агент сбора метрик: опрашивает коллекторы и отправляет снимки на сервер.
type Agent struct {
	cfg            *config.AgentConfig
	collectors     []collector
	httpClient     *http.Client
	jobs           chan job
	logger         *slog.Logger
	retryIntervals []time.Duration
	buffersPool    *pool.Pool[*bytes.Buffer] // переиспользуемые буферы JSON и gzip
	buffersOnce    sync.Once                 // синхронизирует ленивую инициализацию пула
	cryptoKey      *rsa.PublicKey
}

// New создаёт агента с коллекторами runtime- и системных метрик.
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

// buffers возвращает общий для агента пул JSON- и gzip-буферов.
// Инициализация через sync.Once также поддерживает агентов, созданных в тестах
// без конструктора New.
func (a *Agent) buffers() *pool.Pool[*bytes.Buffer] {
	a.buffersOnce.Do(func() {
		a.buffersPool = pool.New(func() *bytes.Buffer { return &bytes.Buffer{} })
	})
	return a.buffersPool
}

// Run запускает сбор и отправку метрик до SIGTERM, SIGINT или SIGQUIT. При
// завершении останавливает опрос, отправляет финальный снимок и дожидается
// отправки всех заданий из очереди.
func (a *Agent) Run() error {
	if err := a.loadCryptoKey(); err != nil {
		return err
	}

	a.logger.Info("Agent is running", "server", a.cfg.ServerAddress, "rate_limit", a.cfg.RateLimit)

	ctx, cancel := signal.NotifyContext(context.Background(),
		syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	defer cancel()

	var workersWG sync.WaitGroup
	var runWG sync.WaitGroup

	// run multiple workers to push metrics; их количество ограничивает число
	// одновременно исходящих запросов на сервер сверху (rate limit).
	a.logger.Info("Start push workers", "count", a.cfg.RateLimit)
	for idx := range a.cfg.RateLimit {
		workersWG.Go(func() {
			a.runWorker(context.Background(), idx)
		})
	}

	// run report metrics goroutine
	runWG.Go(func() {
		a.runReporting(ctx)
	})

	// run poll metrics goroutine — внутри запускает по горутине на каждый
	// коллектор.
	runWG.Go(func() {
		a.runPolling(ctx)
	})

	<-ctx.Done()
	runWG.Wait()

	// Забираем последний снимок после остановки опроса и отчётности.
	a.report(context.Background())
	close(a.jobs)
	workersWG.Wait()

	a.logger.Info("Agent stopped")
	return nil
}

func (a *Agent) loadCryptoKey() error {
	if a.cfg.CryptoKey == "" {
		return nil
	}
	key, err := crypto.LoadPublicKey(a.cfg.CryptoKey)
	if err != nil {
		return err
	}
	a.cryptoKey = key
	return nil
}

func (a *Agent) runReporting(ctx context.Context) {
	timer := time.NewTicker(a.cfg.ReportInterval)
	a.logger.Debug("Reporting process started...", "interval", a.cfg.ReportInterval)
	for {
		select {
		case <-timer.C:
			a.report(ctx)
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

			metrics := make([]models.Metrics, len(j.metrics))
			for i, m := range j.metrics {
				metrics[i] = m.Metric
			}
			if err := a.sendMetricsBatch(ctx, metrics); err != nil {
				log.Error("send metrics batch", "error", err)
				for _, pm := range j.metrics {
					pm.Restore()
				}
			}

		case <-ctx.Done():
			return
		}
	}
}

func (a *Agent) report(ctx context.Context) {
	// снимаем снимки со всех коллекторов параллельно и объединяем результаты
	results := make([][]collectors.PendingMetric, len(a.collectors))
	var wg sync.WaitGroup
	for i, c := range a.collectors {
		wg.Go(func() {
			results[i] = c.Collect()
		})
	}
	wg.Wait()

	// объединяем метрики в один slice
	var pendingMetrics []collectors.PendingMetric
	for _, r := range results {
		pendingMetrics = append(pendingMetrics, r...)
	}

	// если метрик нет, выходим
	if len(pendingMetrics) == 0 {
		return
	}

	select {
	case a.jobs <- job{
		metrics: pendingMetrics,
	}:
	case <-ctx.Done():
		// restore all metrics state
		for _, p := range pendingMetrics {
			p.Restore()
		}
	}

	a.logger.Debug("Metrics successfully scheduled to send")
}

func (a *Agent) sendMetricsBatch(ctx context.Context, metrics []models.Metrics) error {
	buffers := a.buffers()
	buf := buffers.Get()
	// Буфер нельзя вернуть, пока sendData использует его данные для сжатия.
	defer buffers.Put(buf)

	if err := json.NewEncoder(buf).Encode(metrics); err != nil {
		return fmt.Errorf("json encode error: %w", err)
	}
	url := fmt.Sprintf("http://%s/updates", a.cfg.ServerAddress)
	return a.sendData(ctx, url, buf.Bytes())
}

const hashHeader = "HashSHA256"

func (a *Agent) sendData(ctx context.Context, url string, data []byte) error {
	buffers := a.buffers()
	compressed := buffers.Get()
	// Тело запроса ссылается на этот буфер, в том числе при повторных попытках.
	defer buffers.Put(compressed)

	gw := gzip.NewWriter(compressed)
	if _, err := gw.Write(data); err != nil {
		return fmt.Errorf("gzip write error: %w", err)
	}
	if err := gw.Close(); err != nil {
		return fmt.Errorf("gzip close error: %w", err)
	}
	body := compressed.Bytes()
	if a.cryptoKey != nil {
		var err error
		body, err = crypto.Encrypt(a.cryptoKey, body)
		if err != nil {
			return fmt.Errorf("encrypt request body: %w", err)
		}
	}

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
