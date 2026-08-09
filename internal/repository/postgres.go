package repository

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/alexandrovas/go-musthave-metrics/internal/models"
	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	// Register pgx as the database/sql driver
	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/postgres/*.sql
var migrationsFS embed.FS

// PostgresStorage — хранилище метрик на базе PostgreSQL.
type PostgresStorage struct {
	logger *slog.Logger
	db     *sql.DB
}

// NewPostgresStorage открывает пул соединений к PostgreSQL по dsn и сразу же
// накатывает миграции, чтобы после успешного возврата хранилище было готово к работе.
func NewPostgresStorage(ctx context.Context, logger *slog.Logger, dsn string) (*PostgresStorage, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("open database: %w", err)
	}

	s := &PostgresStorage{
		logger: logger,
		db:     db,
	}

	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return s, nil
}

// Close закрывает пул соединений.
func (s *PostgresStorage) Close() {
	s.db.Close()
}

func (s *PostgresStorage) UpdateBatch(ctx context.Context, metrics []models.Metrics) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	for _, m := range metrics {
		switch m.MType {
		case models.Gauge:
			err := s.setGauge(ctx, m.ID, *m.Value, tx)
			if err != nil {
				return err
			}
		case models.Counter:
			err := s.addCounter(ctx, m.ID, *m.Delta, tx)
			if err != nil {
				return err
			}
		}
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func (s *PostgresStorage) SetGauge(ctx context.Context, name string, value float64) error {
	return s.setGauge(ctx, name, value, nil)
}

func (s *PostgresStorage) setGauge(ctx context.Context, name string, value float64, tx *sql.Tx) error {
	const q = `
		INSERT INTO gauges (id, value) VALUES ($1, $2)
		ON CONFLICT (id) DO UPDATE SET value = EXCLUDED.value`
	var err error
	if tx != nil {
		_, err = tx.ExecContext(ctx, q, name, value)
	} else {
		_, err = s.db.ExecContext(ctx, q, name, value)
	}
	if err != nil {
		return fmt.Errorf("set gauge %q: %w", name, err)
	}
	return nil
}

func (s *PostgresStorage) GetGauge(ctx context.Context, name string) (float64, bool, error) {
	const q = `SELECT value FROM gauges WHERE id = $1`
	var v float64
	if err := s.db.QueryRowContext(ctx, q, name).Scan(&v); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("get gauge %q: %w", name, err)
	}
	return v, true, nil
}

func (s *PostgresStorage) AddCounter(ctx context.Context, name string, delta int64) error {
	return s.addCounter(ctx, name, delta, nil)
}

func (s *PostgresStorage) addCounter(ctx context.Context, name string, delta int64, tx *sql.Tx) error {
	const q = `
		INSERT INTO counters (id, delta) VALUES ($1, $2)
		ON CONFLICT (id) DO UPDATE SET delta = counters.delta + EXCLUDED.delta`
	var err error
	if tx != nil {
		_, err = tx.ExecContext(ctx, q, name, delta)
	} else {
		_, err = s.db.ExecContext(ctx, q, name, delta)
	}
	if err != nil {
		return fmt.Errorf("add counter %q: %w", name, err)
	}
	return nil
}

func (s *PostgresStorage) GetCounter(ctx context.Context, name string) (int64, bool, error) {
	const q = `SELECT delta FROM counters WHERE id = $1`
	var d int64
	if err := s.db.QueryRowContext(ctx, q, name).Scan(&d); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("get counter %q: %w", name, err)
	}
	return d, true, nil
}

func (s *PostgresStorage) Gauges(ctx context.Context) (map[string]float64, error) {
	const q = `SELECT id, value FROM gauges`
	rows, err := s.db.QueryContext(ctx, q)
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
	rows, err := s.db.QueryContext(ctx, q)
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
	return s.db.PingContext(ctx)
}

// migrate накатывает все непримененные миграции из каталога `migrations/postgres“.
// golang-migrate v4's blocking API не принимает контекст, поэтому он не используется.
func (s *PostgresStorage) migrate(_ context.Context) error {
	sourceDriver, err := iofs.New(migrationsFS, "migrations/postgres")
	if err != nil {
		return fmt.Errorf("create iofs source: %w", err)
	}
	defer sourceDriver.Close()

	dbDriver, err := pgxmigrate.WithInstance(s.db, &pgxmigrate.Config{})
	if err != nil {
		return fmt.Errorf("create db driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "pgx5", dbDriver)
	if err != nil {
		return fmt.Errorf("initialize migrator: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}

	s.logger.Info("database migrations applied")
	return nil
}
