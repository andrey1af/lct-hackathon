package http

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

const (
	healthStatusOK          = "ok"
	healthStatusUnavailable = "unavailable"

	serviceDatabase = "database"
)

type Pinger interface {
	PingContext(ctx context.Context) error
}

type livenessResponse struct {
	Status string `json:"status"`
}

type readinessResponse struct {
	Status   string            `json:"status"`
	Services map[string]string `json:"services"`
}

type HealthHandler struct {
	db      Pinger
	timeout time.Duration
}

func NewHealthHandler(db Pinger, timeout time.Duration) *HealthHandler {
	return &HealthHandler{db: db, timeout: timeout}
}

func (h *HealthHandler) Liveness(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, livenessResponse{Status: healthStatusOK})
}

func (h *HealthHandler) Readiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)

	defer cancel()

	services := map[string]string{
		serviceDatabase: healthStatusOK,
	}
	status := healthStatusOK

	if err := h.db.PingContext(ctx); err != nil {
		slog.Default().Error("readiness check failed: database unreachable", "error", err)
		services[serviceDatabase] = healthStatusUnavailable
		status = healthStatusUnavailable
	}

	code := http.StatusOK
	if status != healthStatusOK {
		code = http.StatusServiceUnavailable
	}

	writeJSON(w, code, readinessResponse{Status: status, Services: services})
}
