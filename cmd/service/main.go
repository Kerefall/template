// Package main - точка входа: читает конфиг, собирает контейнер, запускает
// его и ждёт сигнала остановки.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Kerefall/template/internal/config"
	"github.com/Kerefall/template/internal/container"
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

	c, err := container.New(ctx, cfg)
	if err != nil {
		return err
	}

	return c.Run(ctx)
}
