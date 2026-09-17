package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"go_pet_project/internal/platform/metrics"
)

type Server struct {
	server *http.Server
	logger *slog.Logger
}

func New(address string, handler http.Handler, logger *slog.Logger) *Server {
	return &Server{
		server: &http.Server{
			Addr:              address,
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      10 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
		logger: logger,
	}
}

func NewRouter(readiness func(context.Context) error, registerRoutes ...func(*http.ServeMux)) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := readiness(ctx); err != nil {
			http.Error(w, "service unavailable\n", http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	mux.Handle("GET /metrics", metrics.Handler())

	for _, register := range registerRoutes {
		register(mux)
	}

	return metrics.HTTPMiddleware(mux)
}

type ReadinessGate struct {
	shuttingDown atomic.Bool
	check        func(context.Context) error
}

func NewReadinessGate(check func(context.Context) error) *ReadinessGate {
	return &ReadinessGate{check: check}
}

func (g *ReadinessGate) Check(ctx context.Context) error {
	if g.shuttingDown.Load() {
		return errors.New("service is shutting down")
	}
	if g.check == nil {
		return nil
	}

	return g.check(ctx)
}

func (g *ReadinessGate) MarkShuttingDown() {
	g.shuttingDown.Store(true)
}

func (s *Server) Start() error {
	s.logger.Info("http server starting", "address", s.server.Addr)

	if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("http server shutdown started")

	if err := s.server.Shutdown(ctx); err != nil {
		return err
	}

	s.logger.Info("http server shutdown completed")

	return nil
}
