package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"go_pet_project/internal/bootstrap"
	"go_pet_project/internal/platform/config"
	"go_pet_project/internal/platform/logging"
	"go_pet_project/internal/platform/migrations"
)

const migrationsDir = "migrations"

func main() {
	logger := logging.New()

	if err := run(logger); err != nil {
		logger.Error("fatal startup error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config loading error", "error", err)
		return err
	}

	if err := migrations.Up(ctx, cfg.DB, migrationsDir, logger); err != nil {
		logger.Error("migration error", "error", err)
		return err
	}

	app, err := bootstrap.NewAPI(ctx, cfg)
	if err != nil {
		return err
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Start()
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
		return app.Shutdown(context.Background())
	case err := <-errCh:
		if err == nil || errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}
}
