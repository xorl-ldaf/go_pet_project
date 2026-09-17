package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"go_pet_project/internal/platform/config"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type Postgres struct {
	GORM *gorm.DB
	SQL  *sql.DB
}

func Open(ctx context.Context, cfg config.DBConfig, logger *slog.Logger) (*Postgres, error) {
	gormDB, err := gorm.Open(postgres.Open(cfg.URL()), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open postgres connection: %w", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("get underlying sql database: %w", err)
	}

	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	logger.Info("database connection established", "database", cfg.RedactedURL())

	return &Postgres{
		GORM: gormDB,
		SQL:  sqlDB,
	}, nil
}

func (p *Postgres) Ping(ctx context.Context) error {
	if p == nil || p.SQL == nil {
		return fmt.Errorf("postgres connection is not initialized")
	}

	return p.SQL.PingContext(ctx)
}

func (p *Postgres) Close() error {
	if p == nil || p.SQL == nil {
		return nil
	}

	return p.SQL.Close()
}
