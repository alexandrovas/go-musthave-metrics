package server

import (
	"context"
	"errors"
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
	// на случай, если ListenAndServe завершился по причине, отличной от сигнала
	// (например, ошибка старта) — разблокируем фоновые горутины и дожидаемся их
	cancel()
	wg.Wait()

	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
