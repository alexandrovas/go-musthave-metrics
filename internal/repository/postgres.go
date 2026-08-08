package repository

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStorage — хранилище метрик на базе PostgreSQL.
// Методы работы с метриками пока не реализованы (заглушки), рабочим
// является только Ping — проверка реального соединения с БД.
type PostgresStorage struct {
	logger *slog.Logger
	pool   *pgxpool.Pool
}

// NewPostgresStorage открывает пул соединений к PostgreSQL по dsn.
// Само соединение устанавливается лениво — при первом запросе (например, Ping).
func NewPostgresStorage(ctx context.Context, logger *slog.Logger, dsn string) (*PostgresStorage, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}

	return &PostgresStorage{
		logger: logger,
		pool:   pool,
	}, nil
}

// Close закрывает пул соединений.
func (s *PostgresStorage) Close() {
	s.pool.Close()
}

func (*PostgresStorage) SetGauge(name string, value float64) {
}

func (*PostgresStorage) GetGauge(name string) (float64, bool) {
	return 0, false
}

func (*PostgresStorage) AddCounter(name string, delta int64) {
}

func (*PostgresStorage) GetCounter(name string) (int64, bool) {
	return 0, false
}

func (*PostgresStorage) Gauges() map[string]float64 {
	return nil
}

func (*PostgresStorage) Counters() map[string]int64 {
	return nil
}

// Ping проверяет доступность базы данных.
func (s *PostgresStorage) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	return s.pool.Ping(ctx)
}
