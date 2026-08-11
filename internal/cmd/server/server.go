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

	"github.com/alexandrovas/go-musthave-metrics/internal/config"
	handlerHttp "github.com/alexandrovas/go-musthave-metrics/internal/handler/http"
	"github.com/alexandrovas/go-musthave-metrics/internal/repository"
	"github.com/alexandrovas/go-musthave-metrics/internal/service"
)

type Server struct {
	cfg    *config.ServerConfig
	logger *slog.Logger
}

func New(cfg *config.ServerConfig, logger *slog.Logger) *Server {
	return &Server{
		cfg:    cfg,
		logger: logger,
	}
}

func (s *Server) Run() error {

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	var wg sync.WaitGroup

	var repo service.Repository
	if s.cfg.DatabaseDSN == "" {
		memStore := repository.NewMemStorage(s.logger)

		if s.cfg.FileStoragePath != "" {

			// Восстановление состояния из файла при старте
			if s.cfg.RestoreState {
				if err := memStore.Load(s.cfg.FileStoragePath); err != nil {
					return fmt.Errorf("cannot restore state: %w", err)
				}
				s.logger.Info("state restored from file", "path", s.cfg.FileStoragePath)
			}

			// Настройка сохранения: синхронное (интервал == 0) или периодическое
			if s.cfg.StoreInterval == 0 {
				memStore.EnableSyncSave(s.cfg.FileStoragePath)
			} else {
				wg.Go(func() {
					memStore.RunPeriodicSave(ctx, s.cfg.FileStoragePath, s.cfg.StoreInterval)
				})
			}
		}
		repo = memStore
	} else {
		pgStore, err := repository.NewPostgresStorage(ctx, s.logger,
			s.cfg.DatabaseDSN)
		if err != nil {
			return fmt.Errorf("connect to database: %w", err)
		}
		defer pgStore.Close()
		repo = pgStore
	}

	// печатаем в консоль текущее состояние хранилища (для дебага)
	// const storageLogInterval = 5 * time.Second
	// wg.Go(func() {
	// 	ticker := time.NewTicker(storageLogInterval)
	// 	defer ticker.Stop()
	// 	for {
	// 		select {
	// 		case <-ticker.C:
	// 			repo.Log()
	// 		case <-ctx.Done():
	// 			return
	// 		}
	// 	}
	// })

	srv := &http.Server{
		Addr:    s.cfg.Address,
		Handler: handlerHttp.NewRouter(repo, s.logger),
	}

	wg.Go(func() {
		<-ctx.Done()
		if err := srv.Shutdown(context.Background()); err != nil {
			s.logger.Error("server shutdown", "error", err)
		}
	})

	s.logger.Info("starting server", "address", s.cfg.Address)
	err := srv.ListenAndServe()
	cancel()
	wg.Wait()

	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
