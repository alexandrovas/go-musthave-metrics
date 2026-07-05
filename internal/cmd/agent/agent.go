package agent

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/alexandrovas/go-musthave-metrics/internal/config"
)

type agent struct {
	cfg        *config.AgentConfig
	collector  *collector
	httpClient *http.Client
	jobs       chan metricValue
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
		jobs: make(chan metricValue, cfg.Workers*10),
	}

	slog.Info("Agent is running", "server", cfg.ServerAddress, "workers", cfg.Workers)

	ctx, cancel := context.WithCancel(ctx)
	// handle Ctrl+C
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	defer func() {
		signal.Stop(c)
		cancel()
	}()

	var wg sync.WaitGroup
	wg.Go(func() {
		select {
		case <-c:
			cancel()
		case <-ctx.Done():
		}
	})

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
		case m, ok := <-a.jobs:
			if !ok {
				return
			}
			if err := a.sendMetric(ctx, m.name, string(m.mtype), m.value); err != nil {
				log.Error("send metric", "type", m.mtype, "name", m.name, "value", m.value, "error", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (a *agent) report(ctx context.Context) {
	for _, m := range a.collector.collect() {
		select {
		case a.jobs <- m:
		case <-ctx.Done():
			return
		}
	}
}

func (a *agent) sendMetric(ctx context.Context, name, mtype, value string) error {
	url := fmt.Sprintf("http://%s/update/%s/%s/%s", a.cfg.ServerAddress, mtype, name, value)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/plain")

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
