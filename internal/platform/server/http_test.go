package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadinessGate(t *testing.T) {
	gate := NewReadinessGate(func(context.Context) error {
		return nil
	})
	router := NewRouter(gate.Check)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("readyz status = %d, want 200", response.Code)
	}

	gate.MarkShuttingDown()
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz during shutdown = %d, want 503", response.Code)
	}
}

func TestReadinessFailure(t *testing.T) {
	router := NewRouter(func(context.Context) error {
		return errors.New("db unavailable")
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz status = %d, want 503", response.Code)
	}
}

func TestHTTPMetricsUseRoutePatternLabel(t *testing.T) {
	router := NewRouter(func(context.Context) error {
		return nil
	}, func(mux *http.ServeMux) {
		mux.HandleFunc("GET /items/{id}", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/items/550e8400-e29b-41d4-a716-446655440000", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("handler status = %d, want 204", response.Code)
	}

	metricsResponse := httptest.NewRecorder()
	router.ServeHTTP(metricsResponse, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := metricsResponse.Body.String()
	if !strings.Contains(body, `route="GET /items/{id}"`) {
		t.Fatalf("metrics missing bounded route label:\n%s", body)
	}
	if strings.Contains(body, "550e8400-e29b-41d4-a716-446655440000") {
		t.Fatalf("metrics leaked raw URL into label:\n%s", body)
	}
}
