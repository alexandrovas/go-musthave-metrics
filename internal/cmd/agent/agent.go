package agent

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/alexandrovas/go-musthave-metrics/internal/config"
	"github.com/alexandrovas/go-musthave-metrics/internal/models"
)

type Agent struct {
	cfg        *config.AgentConfig
	collector  *collector
	httpClient *http.Client
	jobs       chan pendingMetric
	logger     *slog.Logger
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
		jobs:   make(chan pendingMetric, cfg.Workers*10),
		logger: logger,
	}
}

func (a *Agent) Run() error {
	ctx := context.Background()

	a.logger.Info("Agent is running", "server", a.cfg.ServerAddress, "workers", a.cfg.Workers)

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	var wg sync.WaitGroup

	// run multiple workers to push metrics
	for idx := range a.cfg.Workers {
		wg.Go(func() {
			a.runWorker(ctx, idx)
		})
	}

	// run report metrics goroutine
	wg.Go(func() {
		a.runReporting(ctx)
	})

	// run poll metrics goroutine
	wg.Go(func() {
		a.runPolling(ctx)
	})

	wg.Wait()
	a.logger.Info("Agent stopped")
	return nil
}

func (a *Agent) runReporting(ctx context.Context) {
	timer := time.NewTicker(a.cfg.ReportInterval)
	a.logger.Debug("Reporting process started...", "interval", a.cfg.ReportInterval)
	for {
		select {
		case <-timer.C:
			a.report(ctx)
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
		case p, ok := <-a.jobs:
			if !ok {
				return
			}
			if err := a.sendMetric(ctx, p.Metric); err != nil {
				log.Error("send metric", "type", p.Metric.MType, "name", p.Metric.ID, "error", err)
				p.Restore()
			}
		case <-ctx.Done():
			return
		}
	}
}

func (a *Agent) report(ctx context.Context) {
	for _, p := range a.collector.collect() {
		select {
		case a.jobs <- p:
		case <-ctx.Done():
			p.Restore()
			return
		}
	}
}

func (a *Agent) sendMetric(ctx context.Context, metric models.Metrics) error {
	url := fmt.Sprintf("http://%s/update", a.cfg.ServerAddress)

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(metric); err != nil {
		return fmt.Errorf("json encode error: %w", err)
	}

	var compressed bytes.Buffer
	gw := gzip.NewWriter(&compressed)
	if _, err := gw.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("gzip write error: %w", err)
	}
	if err := gw.Close(); err != nil {
		return fmt.Errorf("gzip close error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &compressed)
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %s", resp.Status)
	}
	return nil
}
