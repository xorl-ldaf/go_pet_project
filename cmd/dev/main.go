package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go_pet_project/internal/platform/config"
	"go_pet_project/internal/platform/database"
	"go_pet_project/internal/platform/logging"
	"go_pet_project/internal/platform/migrations"
)

const (
	composeFile   = "deploy/compose.yaml"
	migrationsDir = "migrations"
)

func main() {
	logger := logging.New()

	if err := run(logger); err != nil {
		logger.Error("dev launcher failed", "error", err)
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
	logger.Info("development config loaded", "http_port", cfg.HTTP.Port, "database", cfg.DB.RedactedURL())

	if err := checkDocker(ctx); err != nil {
		return err
	}

	if err := composeUp(ctx, logger); err != nil {
		return err
	}

	if err := waitForPostgres(ctx, cfg.DB, 60*time.Second, time.Second, logger); err != nil {
		return err
	}

	if err := migrations.Up(ctx, cfg.DB, migrationsDir, logger); err != nil {
		return err
	}

	return runAPI(ctx, logger)
}

func checkDocker(ctx context.Context) error {
	if _, err := exec.LookPath("docker"); err != nil {
		return errors.New("Docker CLI not found")
	}

	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(checkCtx, "docker", "info")
	output, err := cmd.CombinedOutput()
	if err != nil {
		details := strings.TrimSpace(string(output))
		if details == "" {
			details = err.Error()
		}
		return fmt.Errorf("Docker daemon is not running or is not accessible: %s", details)
	}

	composeCtx, composeCancel := context.WithTimeout(ctx, 10*time.Second)
	defer composeCancel()

	composeCmd := exec.CommandContext(composeCtx, "docker", "compose", "version")
	composeOutput, err := composeCmd.CombinedOutput()
	if err != nil {
		details := strings.TrimSpace(string(composeOutput))
		if details == "" {
			details = err.Error()
		}
		return fmt.Errorf("Docker Compose plugin is not available: %s", details)
	}

	return nil
}

func composeUp(ctx context.Context, logger *slog.Logger) error {
	logger.Info("starting Docker infrastructure", "compose_file", composeFile)

	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", composeFile, "up", "-d")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose up failed: %w", err)
	}

	logger.Info("Docker infrastructure started")

	return nil
}

func waitForPostgres(ctx context.Context, cfg config.DBConfig, timeout time.Duration, interval time.Duration, logger *slog.Logger) error {
	logger.Info("waiting for PostgreSQL readiness", "timeout", timeout.String(), "interval", interval.String())

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		db, err := database.Open(waitCtx, cfg, logger)
		if err == nil {
			if closeErr := db.Close(); closeErr != nil {
				logger.Warn("close readiness database connection", "error", closeErr)
			}
			logger.Info("PostgreSQL is ready")
			return nil
		}

		select {
		case <-waitCtx.Done():
			return fmt.Errorf("PostgreSQL did not become ready before timeout: %w", waitCtx.Err())
		case <-ticker.C:
		}
	}
}

func runAPI(ctx context.Context, logger *slog.Logger) error {
	logger.Info("starting API process")

	cmd := exec.Command("go", "run", "./cmd/api")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start API process: %w", err)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		logger.Info("stopping API process")
		return stopProcessGroup(cmd, waitCh, logger)
	case err := <-waitCh:
		if err != nil {
			return fmt.Errorf("API process exited: %w", err)
		}
		return nil
	}
}

func stopProcessGroup(cmd *exec.Cmd, waitCh <-chan error, logger *slog.Logger) error {
	if cmd.Process == nil {
		return nil
	}

	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("send SIGTERM to API process group: %w", err)
	}

	select {
	case err := <-waitCh:
		if err != nil {
			logger.Info("API process stopped", "error", err)
		} else {
			logger.Info("API process stopped")
		}
		return nil
	case <-time.After(15 * time.Second):
		logger.Warn("API process did not stop gracefully, sending SIGKILL")
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("send SIGKILL to API process group: %w", err)
		}
		<-waitCh
		return nil
	}
}
