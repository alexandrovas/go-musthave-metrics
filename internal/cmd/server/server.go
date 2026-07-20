package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/alexandrovas/go-musthave-metrics/internal/config"
	handlerHttp "github.com/alexandrovas/go-musthave-metrics/internal/handler/http"
	"github.com/alexandrovas/go-musthave-metrics/internal/repository"
)

const storageLogInterval = 5 * time.Second

func Run(cfg *config.ServerConfig) error {
	repo := repository.NewMemStorage()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	var wg sync.WaitGroup

	if cfg.FileStoragePath != "" {

		// Восстановление состояния из файла при старте
		if cfg.RestoreState {
			if err := repo.Load(cfg.FileStoragePath); err != nil {
				return fmt.Errorf("cannot restore state: %w", err)
			}
			slog.Info("state restored from file", "path", cfg.FileStoragePath)
		}

		// Настройка сохранения: синхронное (интервал == 0) или периодическое
		if cfg.StoreInterval == 0 {
			repo.EnableSyncSave(cfg.FileStoragePath)
		} else {
			wg.Go(func() {
				repo.RunPeriodicSave(ctx, cfg.FileStoragePath, cfg.StoreInterval)
			})
		}
	}

	// печатаем в консоль текущее состояние хранилища (для дебага)
	wg.Go(func() {
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
	})

	srv := &http.Server{
		Addr:    cfg.Address,
		Handler: handlerHttp.NewRouter(repo),
	}

	wg.Go(func() {
		<-ctx.Done()
		if err := srv.Shutdown(context.Background()); err != nil {
			slog.Error("server shutdown", "error", err)
		}
	})

	slog.Info("starting server", "address", cfg.Address)
	err := srv.ListenAndServe()
	cancel()
	wg.Wait()

	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
