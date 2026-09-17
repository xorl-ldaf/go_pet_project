package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"go_pet_project/internal/platform/metrics"
)

type MetricsServer struct {
	server *http.Server
	logger *slog.Logger
}

func NewMetricsServer(port int, logger *slog.Logger) *MetricsServer {
	if logger == nil {
		logger = slog.Default()
	}

	mux := http.NewServeMux()
	mux.Handle("GET /metrics", metrics.Handler())
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	return &MetricsServer{
		server: &http.Server{
			Addr:              net.JoinHostPort("", strconv.Itoa(port)),
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      10 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
		logger: logger.With("component", "metrics_server", "port", port),
	}
}

func (s *MetricsServer) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("metrics server starting", "address", s.server.Addr)
		if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown metrics server: %w", err)
		}
		return nil
	case err := <-errCh:
		return err
	}
}
