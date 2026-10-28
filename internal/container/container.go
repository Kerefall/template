// Package container создаёт все зависимости сервиса и связывает их между
// собой. Это единственное место, где встречаются конкретные реализации:
// пул базы, репозитории, телеметрия. usecase и хендлеры знают только
// интерфейсы.
package container

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Kerefall/template/internal/config"
	"github.com/Kerefall/template/internal/http/health"
	"github.com/Kerefall/template/internal/http/ready"
	"github.com/Kerefall/template/internal/http/router"
	createhttp "github.com/Kerefall/template/internal/http/trip/create"
	finishhttp "github.com/Kerefall/template/internal/http/trip/finish"
	gethttp "github.com/Kerefall/template/internal/http/trip/get"
	"github.com/Kerefall/template/internal/infra/postgres"
	"github.com/Kerefall/template/internal/infra/postgres/transaction"
	"github.com/Kerefall/template/internal/infra/telemetry"
	triprepo "github.com/Kerefall/template/internal/repository/trip"
	createuc "github.com/Kerefall/template/internal/usecase/trip/create"
	finishuc "github.com/Kerefall/template/internal/usecase/trip/finish"
	getuc "github.com/Kerefall/template/internal/usecase/trip/get"
)

type Container struct {
	cfg       config.Config
	logger    *slog.Logger
	telemetry *telemetry.Telemetry
	pool      *pgxpool.Pool
	server    *http.Server
}

// now отдаёт время в UTC с точностью до микросекунд: так оно хранится в PostgreSQL.
func now() time.Time {
	return time.Now().UTC().Truncate(time.Microsecond)
}

func New(ctx context.Context, cfg config.Config) (*Container, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		return nil, fmt.Errorf("invalid LOG_LEVEL: %w", err)
	}
	logger := telemetry.NewLogger(level)

	tel, err := telemetry.Setup(ctx, logger)
	if err != nil {
		return nil, fmt.Errorf("setup telemetry: %w", err)
	}

	pool, err := postgres.NewPool(ctx, postgres.Config{
		URL:             cfg.DatabaseURL,
		MaxConns:        cfg.DatabaseMaxConns,
		MinConns:        cfg.DatabaseMinConns,
		MaxConnLifetime: cfg.DatabaseMaxConnLifetime,
		ConnectTimeout:  cfg.DatabaseConnectTimeout,
	})
	if err != nil {
		_ = tel.Shutdown(context.WithoutCancel(ctx))
		return nil, err
	}

	txManager := transaction.NewManager(pool)
	repo := triprepo.New(pool)

	create := createuc.New(repo, txManager, tel.Metrics, logger, now)
	get := getuc.New(repo)
	finish := finishuc.New(repo, txManager, tel.Metrics, logger, now)

	timeout := cfg.DatabaseQueryTimeout
	createHandler := createhttp.New(create, timeout, logger)
	getHandler := gethttp.New(get, timeout, logger)
	finishHandler := finishhttp.New(finish, timeout, logger)
	healthHandler := health.New()
	readyHandler := ready.New(pool, timeout, logger)

	handler, err := router.New(router.Handlers{
		CreateTrip: createHandler.Handle,
		GetTrip:    getHandler.Handle,
		FinishTrip: finishHandler.Handle,
		Health:     healthHandler.Handle,
		Ready:      readyHandler.Handle,
	}, logger)
	if err != nil {
		pool.Close()
		_ = tel.Shutdown(context.WithoutCancel(ctx))
		return nil, err
	}

	return &Container{
		cfg:       cfg,
		logger:    logger,
		telemetry: tel,
		pool:      pool,
		server: &http.Server{
			Addr:              cfg.HTTPAddr,
			Handler:           handler,
			ReadTimeout:       cfg.HTTPReadTimeout,
			ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
			WriteTimeout:      cfg.HTTPWriteTimeout,
			IdleTimeout:       cfg.HTTPIdleTimeout,
		},
	}, nil
}

// Run запускает HTTP-сервер и работает, пока контекст не отменён (сигнал
// остановки) или сервер не упадёт. После этого останавливает всё в пределах
// SHUTDOWN_TIMEOUT.
func (c *Container) Run(ctx context.Context) error {
	serverErr := make(chan error, 1)
	go func() {
		c.logger.Info("http server starting", "addr", c.cfg.HTTPAddr)

		if err := c.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	var runErr error
	select {
	case err := <-serverErr:
		runErr = fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		c.logger.Info("shutdown signal received")
	}

	return errors.Join(runErr, c.shutdown(ctx))
}

// shutdown: перестаём принимать запросы, ждём текущие, закрываем пул,
// сбрасываем телеметрию. По истечении бюджета выходим принудительно.
func (c *Container) shutdown(ctx context.Context) error {
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.cfg.ShutdownTimeout)
	defer cancel()

	var errs []error

	if err := c.server.Shutdown(shutdownCtx); err != nil {
		c.logger.Error("graceful shutdown failed, closing server forcibly", "error", err)
		_ = c.server.Close()
		errs = append(errs, fmt.Errorf("shutdown http server: %w", err))
	}

	poolClosed := make(chan struct{})
	go func() {
		c.pool.Close()
		close(poolClosed)
	}()

	select {
	case <-poolClosed:
		c.logger.Info("database pool closed")
	case <-shutdownCtx.Done():
		c.logger.Error("shutdown timeout exceeded, exiting forcibly")
		return errors.Join(append(errs, errors.New("shutdown timeout exceeded"))...)
	}

	if err := c.telemetry.Shutdown(shutdownCtx); err != nil {
		c.logger.Warn("telemetry shutdown failed", "error", err)
	}

	c.logger.Info("service stopped")

	return errors.Join(errs...)
}
