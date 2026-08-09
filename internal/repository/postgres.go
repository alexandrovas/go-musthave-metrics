package repository

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/postgres/*.sql
var migrationsFS embed.FS

// PostgresStorage — хранилище метрик на базе PostgreSQL.
type PostgresStorage struct {
	logger *slog.Logger
	pool   *pgxpool.Pool
}

// NewPostgresStorage открывает пул соединений к PostgreSQL по dsn и сразу же
// накатывает миграции, чтобы после успешного возврата хранилище было готово к работе.
func NewPostgresStorage(ctx context.Context, logger *slog.Logger, dsn string) (*PostgresStorage, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}

	s := &PostgresStorage{
		logger: logger,
		pool:   pool,
	}

	if err := s.migrate(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return s, nil
}

// Close закрывает пул соединений.
func (s *PostgresStorage) Close() {
	s.pool.Close()
}

func (s *PostgresStorage) SetGauge(ctx context.Context, name string, value float64) error {
	const q = `
		INSERT INTO gauges (id, value) VALUES ($1, $2)
		ON CONFLICT (id) DO UPDATE SET value = EXCLUDED.value`
	if _, err := s.pool.Exec(ctx, q, name, value); err != nil {
		return fmt.Errorf("set gauge %q: %w", name, err)
	}
	return nil
}

func (s *PostgresStorage) GetGauge(ctx context.Context, name string) (float64, bool, error) {
	const q = `SELECT value FROM gauges WHERE id = $1`
	var v float64
	if err := s.pool.QueryRow(ctx, q, name).Scan(&v); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("get gauge %q: %w", name, err)
	}
	return v, true, nil
}

func (s *PostgresStorage) AddCounter(ctx context.Context, name string, delta int64) error {
	const q = `
		INSERT INTO counters (id, delta) VALUES ($1, $2)
		ON CONFLICT (id) DO UPDATE SET delta = counters.delta + EXCLUDED.delta`
	if _, err := s.pool.Exec(ctx, q, name, delta); err != nil {
		return fmt.Errorf("add counter %q: %w", name, err)
	}
	return nil
}

func (s *PostgresStorage) GetCounter(ctx context.Context, name string) (int64, bool, error) {
	const q = `SELECT delta FROM counters WHERE id = $1`
	var d int64
	if err := s.pool.QueryRow(ctx, q, name).Scan(&d); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("get counter %q: %w", name, err)
	}
	return d, true, nil
}

func (s *PostgresStorage) Gauges(ctx context.Context) (map[string]float64, error) {
	const q = `SELECT id, value FROM gauges`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list gauges: %w", err)
	}
	defer rows.Close()

	result := make(map[string]float64)
	for rows.Next() {
		var id string
		var v float64
		if err := rows.Scan(&id, &v); err != nil {
			return nil, fmt.Errorf("scan gauge row: %w", err)
		}
		result[id] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list gauges: %w", err)
	}
	return result, nil
}

func (s *PostgresStorage) Counters(ctx context.Context) (map[string]int64, error) {
	const q = `SELECT id, delta FROM counters`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list counters: %w", err)
	}
	defer rows.Close()

	result := make(map[string]int64)
	for rows.Next() {
		var id string
		var d int64
		if err := rows.Scan(&id, &d); err != nil {
			return nil, fmt.Errorf("scan counter row: %w", err)
		}
		result[id] = d
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list counters: %w", err)
	}
	return result, nil
}

// Ping проверяет доступность базы данных.
func (s *PostgresStorage) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	return s.pool.Ping(ctx)
}

// migrate накатывает все непримененные миграции из каталога `migrations/postgres“.
// golang-migrate v4's blocking API не принимает контекст, поэтому он не используется.
func (s *PostgresStorage) migrate(_ context.Context) error {
	sourceDriver, err := iofs.New(migrationsFS, "migrations/postgres")
	if err != nil {
		return fmt.Errorf("create iofs source: %w", err)
	}

	sqlDB := stdlib.OpenDBFromPool(s.pool)

	dbDriver, err := pgxmigrate.WithInstance(sqlDB, &pgxmigrate.Config{})
	if err != nil {
		return fmt.Errorf("create db driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "pgx5", dbDriver)
	if err != nil {
		return fmt.Errorf("initialize migrator: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}

	s.logger.Info("database migrations applied")
	return nil
}
