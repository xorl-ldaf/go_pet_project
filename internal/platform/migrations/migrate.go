package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"go_pet_project/internal/platform/config"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
)

func Up(ctx context.Context, cfg config.DBConfig, migrationsDir string, logger *slog.Logger) error {
	db, err := sql.Open("postgres", cfg.URL())
	if err != nil {
		return fmt.Errorf("open migration database connection: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping migration database connection: %w", err)
	}

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("create migration database driver: %w", err)
	}

	absDir, err := filepath.Abs(migrationsDir)
	if err != nil {
		return fmt.Errorf("resolve migrations directory: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance("file://"+filepath.ToSlash(absDir), "postgres", driver)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			logger.Info("no migrations found", "path", absDir)
			return nil
		}
		return fmt.Errorf("create migrator: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil {
		switch {
		case errors.Is(err, migrate.ErrNoChange):
			logger.Info("migrations already up to date")
			return nil
		case errors.Is(err, os.ErrNotExist):
			logger.Info("no migrations found", "path", absDir)
			return nil
		default:
			return fmt.Errorf("apply migrations: %w", err)
		}
	}

	logger.Info("migrations applied")

	return nil
}

func Down(ctx context.Context, cfg config.DBConfig, migrationsDir string, logger *slog.Logger) error {
	db, err := sql.Open("postgres", cfg.URL())
	if err != nil {
		return fmt.Errorf("open migration database connection: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping migration database connection: %w", err)
	}

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("create migration database driver: %w", err)
	}

	absDir, err := filepath.Abs(migrationsDir)
	if err != nil {
		return fmt.Errorf("resolve migrations directory: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance("file://"+filepath.ToSlash(absDir), "postgres", driver)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			logger.Info("no migrations found", "path", absDir)
			return nil
		}
		return fmt.Errorf("create migrator: %w", err)
	}
	defer m.Close()

	if err := m.Down(); err != nil {
		switch {
		case errors.Is(err, migrate.ErrNoChange):
			logger.Info("migrations already down")
			return nil
		case errors.Is(err, os.ErrNotExist):
			logger.Info("no migrations found", "path", absDir)
			return nil
		default:
			return fmt.Errorf("rollback migrations: %w", err)
		}
	}

	logger.Info("migrations rolled back")

	return nil
}
