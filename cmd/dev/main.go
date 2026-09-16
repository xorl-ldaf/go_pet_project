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
	"sync"
	"syscall"
	"time"

	"go_pet_project/internal/platform/config"
	"go_pet_project/internal/platform/database"
	"go_pet_project/internal/platform/logging"
	"go_pet_project/internal/platform/migrations"

	"github.com/twmb/franz-go/pkg/kgo"
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

	cfg, err := config.LoadDev()
	if err != nil {
		logger.Error("config loading error", "error", err)
		return err
	}
	logger.Info("development config loaded", "http_port", cfg.HTTP.Port, "database", cfg.DB.RedactedURL())

	if err := checkDocker(ctx); err != nil {
		return err
	}

	if err := composeUp(ctx, logger, "postgres", "kafka"); err != nil {
		return err
	}

	if err := waitForPostgres(ctx, cfg.DB, 60*time.Second, time.Second, logger); err != nil {
		return err
	}

	if err := waitForKafka(ctx, cfg.Kafka.Brokers, 90*time.Second, time.Second, logger); err != nil {
		return err
	}

	if err := migrations.Up(ctx, cfg.DB, migrationsDir, logger); err != nil {
		return err
	}

	return runDevProcesses(ctx, logger)
}

func checkDocker(ctx context.Context) error {
	if _, err := exec.LookPath("docker"); err != nil {
		return errors.New("docker CLI not found")
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
		return fmt.Errorf("docker daemon is not running or is not accessible: %s", details)
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
		return fmt.Errorf("docker Compose plugin is not available: %s", details)
	}

	return nil
}

func composeUp(ctx context.Context, logger *slog.Logger, services ...string) error {
	logger.Info("starting Docker infrastructure", "compose_file", composeFile)

	args := []string{"compose", "-f", composeFile, "up", "-d"}
	args = append(args, services...)
	cmd := exec.CommandContext(ctx, "docker", args...)
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

func waitForKafka(ctx context.Context, brokers []string, timeout time.Duration, interval time.Duration, logger *slog.Logger) error {
	logger.Info("waiting for Kafka readiness", "timeout", timeout.String(), "interval", interval.String(), "brokers", brokers)

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		client, err := kgo.NewClient(kgo.SeedBrokers(brokers...))
		if err == nil {
			pingCtx, pingCancel := context.WithTimeout(waitCtx, 5*time.Second)
			err = client.Ping(pingCtx)
			pingCancel()
			client.Close()
			if err == nil {
				logger.Info("Kafka is ready")
				return nil
			}
		}

		select {
		case <-waitCtx.Done():
			return fmt.Errorf("kafka did not become ready before timeout: %w", waitCtx.Err())
		case <-ticker.C:
		}
	}
}

type devProcess struct {
	name string
	cmd  *exec.Cmd
	done chan struct{}
	mu   sync.Mutex
	err  error
}

func runDevProcesses(ctx context.Context, logger *slog.Logger) error {
	processes := []*devProcess{
		{name: "API", cmd: newGoRunCommand("./cmd/api")},
		{name: "Scheduler", cmd: newGoRunCommand("./cmd/scheduler", "METRICS_PORT=9091")},
		{name: "Notifier", cmd: newGoRunCommand("./cmd/notifier", "METRICS_PORT=9093")},
	}

	for _, process := range processes {
		logger.Info("starting dev process", "process", process.name)
		if err := process.cmd.Start(); err != nil {
			stopDevProcesses(processes, logger)
			return fmt.Errorf("start %s process: %w", process.name, err)
		}
		process.done = make(chan struct{})
		go func(process *devProcess) {
			err := process.cmd.Wait()
			process.mu.Lock()
			process.err = err
			process.mu.Unlock()
			close(process.done)
		}(process)
	}

	type processExit struct {
		process *devProcess
		err     error
	}
	exits := make(chan processExit, len(processes))
	for _, process := range processes {
		go func(process *devProcess) {
			<-process.done
			exits <- processExit{process: process, err: process.waitErr()}
		}(process)
	}

	select {
	case <-ctx.Done():
		logger.Info("stopping dev processes")
		return stopDevProcesses(processes, logger)
	case exit := <-exits:
		if exit.err != nil {
			stopDevProcesses(processes, logger)
			return fmt.Errorf("%s process exited: %w", exit.process.name, exit.err)
		}
		stopDevProcesses(processes, logger)
		return nil
	}
}

func newGoRunCommand(pkg string, extraEnv ...string) *exec.Cmd {
	cmd := exec.Command("go", "run", pkg)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	return cmd
}

func stopDevProcesses(processes []*devProcess, logger *slog.Logger) error {
	var stopErr error
	for _, process := range processes {
		if process.cmd == nil || process.done == nil {
			continue
		}
		if err := stopProcessGroup(process, logger); err != nil && stopErr == nil {
			stopErr = err
		}
	}

	return stopErr
}

func (p *devProcess) waitErr() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.err
}

func stopProcessGroup(process *devProcess, logger *slog.Logger) error {
	if process.cmd.Process == nil {
		return nil
	}

	select {
	case <-process.done:
		return nil
	default:
	}

	if err := syscall.Kill(-process.cmd.Process.Pid, syscall.SIGINT); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("send SIGINT to %s process group: %w", process.name, err)
	}

	select {
	case <-process.done:
		err := process.waitErr()
		if err != nil {
			logger.Info("dev process stopped", "process", process.name, "error", err)
		} else {
			logger.Info("dev process stopped", "process", process.name)
		}
		return nil
	case <-time.After(15 * time.Second):
		logger.Warn("dev process did not stop gracefully, sending SIGKILL", "process", process.name)
		if err := syscall.Kill(-process.cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("send SIGKILL to %s process group: %w", process.name, err)
		}
		<-process.done
		return nil
	}
}
