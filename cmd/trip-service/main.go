package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Kerefall/template/internal/config"
	api "github.com/Kerefall/template/internal/generated"
	"github.com/Kerefall/template/internal/handler"
	"github.com/Kerefall/template/internal/repository"
	"github.com/Kerefall/template/internal/txmanager"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "trip-service:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		return fmt.Errorf("invalid LOG_LEVEL: %w", err)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})).
		With("service", "trip-service")

	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	poolConfig.MaxConns = cfg.DatabaseMaxConns
	poolConfig.MinConns = cfg.DatabaseMinConns
	poolConfig.MaxConnLifetime = cfg.DatabaseMaxConnLifetime
	poolConfig.ConnConfig.ConnectTimeout = cfg.DatabaseConnectTimeout

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return fmt.Errorf("create pool: %w", err)
	}

	// Без базы сервис не стартует
	pingCtx, cancelPing := context.WithTimeout(ctx, cfg.DatabaseConnectTimeout)
	err = pool.Ping(pingCtx)
	cancelPing()
	if err != nil {
		pool.Close()
		return fmt.Errorf("ping database: %w", err)
	}

	h := handler.Handler{
		DB:           pool,
		Trips:        repository.TripRepository{Pool: pool},
		Tx:           txmanager.New(pool),
		QueryTimeout: cfg.DatabaseQueryTimeout,
		Log:          logger,
	}

	router := chi.NewRouter()
	router.NotFound(handler.NotFound)
	router.MethodNotAllowed(handler.MethodNotAllowed)
	api.HandlerWithOptions(h, api.ChiServerOptions{
		BaseRouter:       router,
		ErrorHandlerFunc: handler.BindErrorHandler,
	})

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadTimeout:       cfg.HTTPReadTimeout,
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("http server starting", "addr", cfg.HTTPAddr)

		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		pool.Close()
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	// Весь останов укладываем в SHUTDOWN_TIMEOUT: сначала перестаём принимать
	// запросы и ждём текущие, потом закрываем пул
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	shutdownErr := server.Shutdown(shutdownCtx)
	if shutdownErr != nil {
		logger.Error("graceful shutdown failed, closing server forcibly", "error", shutdownErr)
		_ = server.Close()
	}

	poolClosed := make(chan struct{})
	go func() {
		pool.Close()
		close(poolClosed)
	}()

	select {
	case <-poolClosed:
		logger.Info("database pool closed")
	case <-shutdownCtx.Done():
		logger.Error("shutdown timeout exceeded, exiting forcibly")
		return errors.New("shutdown timeout exceeded")
	}

	if shutdownErr != nil {
		return fmt.Errorf("shutdown http server: %w", shutdownErr)
	}

	logger.Info("service stopped")
	return nil
}
