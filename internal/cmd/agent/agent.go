package agent

import (
	"bytes"
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

type agent struct {
	cfg        *config.AgentConfig
	collector  *collector
	httpClient *http.Client
	jobs       chan pendingMetric
}

func Run(cfg *config.AgentConfig) error {
	ctx := context.Background()

	agent := &agent{
		cfg: cfg,
		collector: &collector{
			counters: make(map[string]int64),
			gauges:   make(map[string]float64),
		},
		httpClient: &http.Client{
			Timeout: time.Second * 5,
		},
		jobs: make(chan pendingMetric, cfg.Workers*10),
	}

	slog.Info("Agent is running", "server", cfg.ServerAddress, "workers", cfg.Workers)

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	var wg sync.WaitGroup

	// run multiple workers to push metrics
	for idx := range cfg.Workers {
		wg.Go(func() {
			agent.runWorker(ctx, idx)
		})
	}

	// run report metrics goroutine
	wg.Go(func() {
		agent.runReporting(ctx)
	})

	// run poll metrics goroutine
	wg.Go(func() {
		agent.runPolling(ctx)
	})

	wg.Wait()
	slog.Info("Agent stopped")
	return nil
}

func (a *agent) runReporting(ctx context.Context) {
	timer := time.NewTicker(a.cfg.ReportInterval)
	slog.Debug("Reporting process started...", "interval", a.cfg.ReportInterval)
	for {
		select {
		case <-timer.C:
			a.report(ctx)
			slog.Debug("Metrics successfully published")

		case <-ctx.Done():
			timer.Stop()
			slog.Debug("Reporting stopped")
			return
		}
	}
}

func (a *agent) runPolling(ctx context.Context) {
	timer := time.NewTicker(a.cfg.PollInterval)
	slog.Debug("Polling process started...", "interval", a.cfg.PollInterval)
	for {
		select {
		case <-timer.C:
			a.collector.poll()
			slog.Debug("Metrics successfully polled")

		case <-ctx.Done():
			timer.Stop()
			slog.Debug("Polling stopped")
			return
		}
	}
}

func (a *agent) runWorker(ctx context.Context, idx uint16) {
	log := slog.With("worker", idx)
	for {
		select {
		case p, ok := <-a.jobs:
			if !ok {
				return
			}
			m := p.Metric
			if err := a.sendMetric(ctx, m); err != nil {
				log.Error("send metric", "type", m.MType, "name", m.ID, "error", err)
				p.Restore()
			}
		case <-ctx.Done():
			return
		}
	}
}

func (a *agent) report(ctx context.Context) {
	for _, p := range a.collector.collect() {
		select {
		case a.jobs <- p:
		case <-ctx.Done():
			p.Restore()
			return
		}
	}
}

func (a *agent) sendMetric(ctx context.Context, metric models.Metrics) error {
	url := fmt.Sprintf("http://%s/update", a.cfg.ServerAddress)

	body := new(bytes.Buffer)
	err := json.NewEncoder(body).Encode(metric)
	if err != nil {
		return fmt.Errorf("json encode error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

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
