package server

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"sync"
	"syscall"

	"github.com/alexandrovas/go-musthave-metrics/internal/audit"
	"github.com/alexandrovas/go-musthave-metrics/internal/config"
	"github.com/alexandrovas/go-musthave-metrics/internal/crypto"
	handlerHttp "github.com/alexandrovas/go-musthave-metrics/internal/handler/http"
	"github.com/alexandrovas/go-musthave-metrics/internal/repository"
	"github.com/alexandrovas/go-musthave-metrics/internal/service"
)

// Компиляционная проверка: реализации хранилищ удовлетворяют service.Repository.
var (
	_ service.Repository = (*repository.MemStorage)(nil)
	_ service.Repository = (*repository.PostgresStorage)(nil)
	_ stateSaver         = (*repository.MemStorage)(nil)
)

// stateSaver — узкий интерфейс для хранилищ, которым нужна финальная запись
// состояния при остановке сервера.
type stateSaver interface {
	Save() error
}

// Server — сервер метрик: поднимает хранилище и HTTP-обработчики, слушает порт.
type Server struct {
	cfg    *config.ServerConfig
	logger *slog.Logger
}

// New создаёт сервер метрик с заданной конфигурацией.
func New(cfg *config.ServerConfig, logger *slog.Logger) *Server {
	return &Server{
		cfg:    cfg,
		logger: logger,
	}
}

// auditObservers собирает наблюдателей аудита (файловый и/или HTTP), включённых
// в конфигурации.
func (s *Server) auditObservers() []audit.Observer {
	var observers []audit.Observer
	if s.cfg.AuditFile != "" {
		observers = append(observers, audit.NewFileObserver(s.cfg.AuditFile))
	}
	if s.cfg.AuditURL != "" {
		observers = append(observers, audit.NewHTTPObserver(s.cfg.AuditURL))
	}
	return observers
}

// Run запускает сервер: инициализирует хранилище, настраивает роутер и слушает
// адрес из конфигурации до SIGTERM, SIGINT или SIGQUIT. При завершении дожидается
// HTTP-обработчиков и сохраняет финальное состояние хранилища.
func (s *Server) Run() error {
	var cryptoKey *rsa.PrivateKey
	if s.cfg.CryptoKey != "" {
		var err error
		cryptoKey, err = crypto.LoadPrivateKey(s.cfg.CryptoKey)
		if err != nil {
			return fmt.Errorf("load decryption key: %w", err)
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(),
		syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	defer cancel()

	var wg sync.WaitGroup

	var repo service.Repository
	if s.cfg.DatabaseDSN == "" {
		memStore := repository.NewMemStorage(s.logger, s.cfg.FileStoragePath)

		if s.cfg.FileStoragePath != "" {

			// Восстановление состояния из файла при старте
			if s.cfg.RestoreState {
				if err := memStore.Load(); err != nil {
					return fmt.Errorf("cannot restore state: %w", err)
				}
				s.logger.Info("state restored from file", "path", s.cfg.FileStoragePath)
			}

			// Настройка сохранения: синхронное (интервал == 0) или периодическое
			if s.cfg.StoreInterval == 0 {
				memStore.EnableSyncSave()
			} else {
				wg.Go(func() {
					memStore.RunPeriodicSave(ctx, s.cfg.StoreInterval)
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

	auditor := audit.NewDispatcher(s.logger, s.cfg.AuditWorkers,
		s.cfg.AuditQueueSize, s.auditObservers()...)

	srv := &http.Server{
		Addr: s.cfg.Address,
		Handler: handlerHttp.NewRouter(repo, s.logger, auditor,
			handlerHttp.WithHashKey(s.cfg.Key),
			handlerHttp.WithCryptoKey(cryptoKey),
		),
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

	// Сохраняем финальное состояние после завершения всех HTTP-обработчиков.
	var saveErr error
	if saver, ok := repo.(stateSaver); ok {
		if err := saver.Save(); err != nil {
			saveErr = fmt.Errorf("save final state: %w", err)
			s.logger.Error("final save failed", "error", err)
		}
	}

	// К этому моменту сервер уже остановлен: Shutdown дождался завершения всех
	// активных HTTP-обработчиков, значит все синхронные вызовы notifyAudit тоже
	// завершились. Создаем отдельный контекст с таймаутом 10 сек, чтоб отправить все события
	// аудита из очереди.
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), audit.DefaultDrainTimeout)
	defer cancelShutdown()
	if closeErr := auditor.Close(shutdownCtx); closeErr != nil {
		s.logger.Error("audit dispatcher close", "error", closeErr)
	}

	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	if saveErr != nil {
		return saveErr
	}
	return nil
}
