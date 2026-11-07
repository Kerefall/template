// Package postgres - пул соединений pgx с телеметрией.
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/multitracer"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/metric/noop"
)

type Config struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	ConnectTimeout  time.Duration
}

// NewPool создаёт пул и проверяет, что база отвечает.
// Телеметрия подключена на уровне пула, репозитории про неё не знают.
func NewPool(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}

	poolConfig.MaxConns = cfg.MaxConns
	poolConfig.MinConns = cfg.MinConns
	poolConfig.MaxConnLifetime = cfg.MaxConnLifetime
	poolConfig.ConnConfig.ConnectTimeout = cfg.ConnectTimeout

	metricsTracer, err := newQueryMetricsTracer()
	if err != nil {
		return nil, err
	}

	// Спан на каждый запрос создаёт otelpgx, а наш трейсер дописывает в него
	// атрибуты db.* и записывает метрику длительности. Собственные метрики
	// otelpgx отключены (noop), иначе db.client.operation.duration считалась
	// бы дважды.
	poolConfig.ConnConfig.Tracer = multitracer.New(
		otelpgx.NewTracer(
			otelpgx.WithTrimSQLInSpanName(),
			otelpgx.WithMeterProvider(noop.NewMeterProvider()),
		),
		metricsTracer,
	)

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return pool, nil
}
