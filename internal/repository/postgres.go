package repository

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/alexandrovas/go-musthave-metrics/internal/models"
	"github.com/alexandrovas/go-musthave-metrics/internal/retry"
	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"

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

// isRetriableDBError сообщает, стоит ли повторить операцию с БД
func isRetriableDBError(err error) bool {
	if err == nil {
		return false
	}

	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		// условия взяты из урока "Интроспекция ошибок"
		if pgerrcode.IsConnectionException(pgErr.Code) ||
			pgerrcode.IsTransactionRollback(pgErr.Code) ||
			pgerrcode.IsInsufficientResources(pgErr.Code) {
			return true
		}
		if pgErr.Code == pgerrcode.CannotConnectNow {
			return true
		}
		return false
	}

	_, ok := errors.AsType[net.Error](err)
	return ok
}

func (s *PostgresStorage) UpdateBatch(ctx context.Context, metrics []models.Metrics) error {
	return retry.Do(ctx, isRetriableDBError, retry.Intervals,
		func() error {
			tx, err := s.db.BeginTx(ctx, nil)
			if err != nil {
				return fmt.Errorf("begin transaction: %w", err)
			}
			defer tx.Rollback()

			for _, m := range metrics {
				switch m.MType {
				case models.Gauge:
					if err := s.setGauge(ctx, m.ID, *m.Value, tx); err != nil {
						return err
					}
				case models.Counter:
					if err := s.addCounter(ctx, m.ID, *m.Delta, tx); err != nil {
						return err
					}
				}
			}

			if err := tx.Commit(); err != nil {
				return fmt.Errorf("commit transaction: %w", err)
			}
			return nil
		})
}

func (s *PostgresStorage) SetGauge(ctx context.Context, name string, value float64) error {
	return retry.Do(ctx, isRetriableDBError, retry.Intervals,
		func() error {
			return s.setGauge(ctx, name, value, nil)
		})
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
	var v float64
	var ok bool
	err := retry.Do(ctx, isRetriableDBError, retry.Intervals,
		func() error {
			const q = `SELECT value FROM gauges WHERE id = $1`
			scanErr := s.db.QueryRowContext(ctx, q, name).Scan(&v)
			switch {
			case scanErr == nil:
				ok = true
				return nil
			case errors.Is(scanErr, sql.ErrNoRows):
				ok = false
				return nil
			default:
				return scanErr
			}
		})
	if err != nil {
		return 0, false, fmt.Errorf("get gauge %q: %w", name, err)
	}
	return v, ok, nil
}

func (s *PostgresStorage) AddCounter(ctx context.Context, name string, delta int64) error {
	return retry.Do(ctx, isRetriableDBError, retry.Intervals,
		func() error {
			return s.addCounter(ctx, name, delta, nil)
		})
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
	var d int64
	var ok bool
	err := retry.Do(ctx, isRetriableDBError, retry.Intervals,
		func() error {
			const q = `SELECT delta FROM counters WHERE id = $1`
			scanErr := s.db.QueryRowContext(ctx, q, name).Scan(&d)
			switch {
			case scanErr == nil:
				ok = true
				return nil
			case errors.Is(scanErr, sql.ErrNoRows):
				ok = false
				return nil
			default:
				return scanErr
			}
		})
	if err != nil {
		return 0, false, fmt.Errorf("get counter %q: %w", name, err)
	}
	return d, ok, nil
}

func (s *PostgresStorage) Gauges(ctx context.Context) (map[string]float64, error) {
	result := make(map[string]float64)
	err := retry.Do(ctx, isRetriableDBError, retry.Intervals,
		func() error {
			const q = `SELECT id, value FROM gauges`
			rows, err := s.db.QueryContext(ctx, q)
			if err != nil {
				return err
			}
			defer rows.Close()

			clear(result) // на случай повторной попытки после частично прочитанного результата
			for rows.Next() {
				var id string
				var v float64
				if err := rows.Scan(&id, &v); err != nil {
					return err
				}
				result[id] = v
			}
			return rows.Err()
		})
	if err != nil {
		return nil, fmt.Errorf("list gauges: %w", err)
	}
	return result, nil
}

func (s *PostgresStorage) Counters(ctx context.Context) (map[string]int64, error) {
	result := make(map[string]int64)
	err := retry.Do(ctx, isRetriableDBError, retry.Intervals,
		func() error {
			const q = `SELECT id, delta FROM counters`
			rows, err := s.db.QueryContext(ctx, q)
			if err != nil {
				return err
			}
			defer rows.Close()

			clear(result)
			for rows.Next() {
				var id string
				var d int64
				if err := rows.Scan(&id, &d); err != nil {
					return err
				}
				result[id] = d
			}
			return rows.Err()
		})
	if err != nil {
		return nil, fmt.Errorf("list counters: %w", err)
	}
	return result, nil
}

// Ping проверяет доступность базы данных.
func (s *PostgresStorage) Ping(ctx context.Context) error {
	return retry.Do(ctx, isRetriableDBError, retry.Intervals,
		func() error {
			pingCtx, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			return s.db.PingContext(pingCtx)
		})
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
