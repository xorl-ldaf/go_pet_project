package logging

import (
	"log/slog"
	"os"
)

func New() *slog.Logger {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	return logger
}
