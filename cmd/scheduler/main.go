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
)

func main() {
	logger := logging.New()

	if err := run(logger); err != nil {
		logger.Error("fatal scheduler error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.LoadScheduler()
	if err != nil {
		logger.Error("config loading error", "error", err)
		return err
	}

	app, err := bootstrap.NewScheduler(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() {
		if err := app.Shutdown(context.Background()); err != nil {
			logger.Error("scheduler shutdown failed", "error", err)
		}
	}()

	err = app.Run(ctx)
	if err == nil || errors.Is(err, context.Canceled) {
		return nil
	}

	return err
}
