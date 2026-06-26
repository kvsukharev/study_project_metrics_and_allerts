package storage

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/model"
)

type PostgresStorage struct {
	pool *pgxpool.Pool
}

func NewPostgresStorage(ctx context.Context, dsn string) (*PostgresStorage, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	s := &PostgresStorage{pool: pool}
	if err := s.createTables(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("create tables: %w", err)
	}
	return s, nil
}

func (p *PostgresStorage) createTables(ctx context.Context) error {
	_, err := p.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS gauges (
			name VARCHAR(255) PRIMARY KEY,
			value DOUBLE PRECISION NOT NULL
		);
		CREATE TABLE IF NOT EXISTS counters (
			name VARCHAR(255) PRIMARY KEY,
			value BIGINT NOT NULL DEFAULT 0
		);
	`)
	return err
}

func (p *PostgresStorage) Ping(ctx context.Context) error {
	return p.pool.Ping(ctx)
}

func (p *PostgresStorage) Close() error {
	p.pool.Close()
	return nil
}

func (p *PostgresStorage) UpdateGauge(name string, value float64) {
	err := withRetry(func() error {
		_, err := p.pool.Exec(context.Background(),
			`INSERT INTO gauges (name, value) VALUES ($1, $2)
			ON CONFLICT (name) DO UPDATE SET value = EXCLUDED.value`,
			name, value)
		return err
	})
	if err != nil {
		log.Printf("UpdateGauge %s: %v", name, err)
	}
}

func (p *PostgresStorage) UpdateCounter(name string, value int64) {
	err := withRetry(func() error {
		_, err := p.pool.Exec(context.Background(),
			`INSERT INTO counters (name, value) VALUES ($1, $2)
			ON CONFLICT (name) DO UPDATE SET value = counters.value + EXCLUDED.value`,
			name, value)
		return err
	})
	if err != nil {
		log.Printf("UpdateCounter %s: %v", name, err)
	}
}

func (p *PostgresStorage) GetGauge(name string) (float64, error) {
	var value float64
	err := p.pool.QueryRow(context.Background(),
		`SELECT value FROM gauges WHERE name = $1`, name).Scan(&value)
	if err != nil {
		return 0, ErrMetricNotFound
	}
	return value, nil
}

func (p *PostgresStorage) GetCounter(name string) (int64, error) {
	var value int64
	err := p.pool.QueryRow(context.Background(),
		`SELECT value FROM counters WHERE name = $1`, name).Scan(&value)
	if err != nil {
		return 0, ErrMetricNotFound
	}
	return value, nil
}

func (p *PostgresStorage) GetAllMetrics() (map[string]float64, map[string]int64) {
	ctx := context.Background()
	gauges := make(map[string]float64)
	counters := make(map[string]int64)

	rows, err := p.pool.Query(ctx, "SELECT name, value FROM gauges")
	if err == nil {
		for rows.Next() {
			var name string
			var value float64
			if rows.Scan(&name, &value) == nil {
				gauges[name] = value
			}
		}
		rows.Close()
	}

	rows, err = p.pool.Query(ctx, "SELECT name, value FROM counters")
	if err == nil {
		for rows.Next() {
			var name string
			var value int64
			if rows.Scan(&name, &value) == nil {
				counters[name] = value
			}
		}
		rows.Close()
	}

	return gauges, counters
}

func (p *PostgresStorage) BatchUpdate(ctx context.Context, metrics []model.Metrics) error {
	delays := []time.Duration{1, 3, 5}
	for attempt := 0; attempt <= len(delays); attempt++ {
		err := p.batchUpdateTx(ctx, metrics)
		if err == nil {
			return nil
		}
		if !isRetriableDBError(err) || attempt == len(delays) {
			return fmt.Errorf("batch update: %w", err)
		}
		time.Sleep(delays[attempt] * time.Second)
	}
	return nil
}

func (p *PostgresStorage) batchUpdateTx(ctx context.Context, metrics []model.Metrics) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	const counterQuery = `INSERT INTO counters (name, value) VALUES ($1, $2)
		ON CONFLICT (name) DO UPDATE SET value = counters.value + EXCLUDED.value`
	const gaugeQuery = `INSERT INTO gauges (name, value) VALUES ($1, $2)
		ON CONFLICT (name) DO UPDATE SET value = EXCLUDED.value`

	for _, m := range metrics {
		switch m.MType {
		case model.TypeCounter:
			if m.Delta == nil {
				continue
			}
			if _, err = tx.Exec(ctx, counterQuery, m.ID, *m.Delta); err != nil {
				return fmt.Errorf("counter update: %w", err)
			}
		case model.TypeGauge:
			if m.Value == nil {
				continue
			}
			if _, err = tx.Exec(ctx, gaugeQuery, m.ID, *m.Value); err != nil {
				return fmt.Errorf("gauge update: %w", err)
			}
		default:
			return fmt.Errorf("unknown metric type: %s", m.MType)
		}
	}

	return tx.Commit(ctx)
}

func withRetry(fn func() error) error {
	delays := []time.Duration{1, 3, 5}
	for attempt := 0; attempt <= len(delays); attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		if !isRetriableDBError(err) || attempt == len(delays) {
			return err
		}
		time.Sleep(delays[attempt] * time.Second)
	}
	return nil
}

func isRetriableDBError(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case pgerrcode.ConnectionException,
			pgerrcode.ConnectionDoesNotExist,
			pgerrcode.ConnectionFailure,
			pgerrcode.SQLClientUnableToEstablishSQLConnection:
			return true
		}
	}
	return false
}
