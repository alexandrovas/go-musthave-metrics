package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/alexandrovas/go-musthave-metrics/internal/config"
	handlerHttp "github.com/alexandrovas/go-musthave-metrics/internal/handler/http"
	"github.com/alexandrovas/go-musthave-metrics/internal/repository"
)

const storageLogInterval = 5 * time.Second

func Run(cfg *config.Config) error {
	repo := repository.NewMemStorage()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	defer signal.Stop(c)
	go func() {
		<-c
		cancel()
	}()

	go func() {
		ticker := time.NewTicker(storageLogInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				repo.Log()
			case <-ctx.Done():
				return
			}
		}
	}()

	srv := &http.Server{
		Addr:    cfg.Server.Address,
		Handler: handlerHttp.NewRouter(repo),
	}

	go func() {
		<-ctx.Done()
		if err := srv.Shutdown(context.Background()); err != nil {
			slog.Error("server shutdown", "error", err)
		}
	}()

	slog.Info("starting server", "address", cfg.Server.Address)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
