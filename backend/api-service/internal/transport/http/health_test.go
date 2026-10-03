package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakePinger struct {
	err error
}

func (f fakePinger) PingContext(_ context.Context) error {
	return f.err
}

func TestHealthHandler_Liveness(t *testing.T) {
	h := NewHealthHandler(fakePinger{}, time.Second)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health/live", nil)

	h.Liveness(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body livenessResponse

	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if body.Status != healthStatusOK {
		t.Errorf("status = %q, want %q", body.Status, healthStatusOK)
	}
}

func TestHealthHandler_Readiness(t *testing.T) {
	tests := []struct {
		name         string
		pingErr      error
		wantCode     int
		wantStatus   string
		wantDBStatus string
	}{
		{
			name:         "database reachable",
			pingErr:      nil,
			wantCode:     http.StatusOK,
			wantStatus:   healthStatusOK,
			wantDBStatus: healthStatusOK,
		},
		{
			name:         "database unreachable",
			pingErr:      errors.New("connection refused"),
			wantCode:     http.StatusServiceUnavailable,
			wantStatus:   healthStatusUnavailable,
			wantDBStatus: healthStatusUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHealthHandler(fakePinger{err: tt.pingErr}, time.Second)

			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health/ready", nil)

			h.Readiness(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantCode)
			}

			var body readinessResponse

			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}

			if body.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", body.Status, tt.wantStatus)
			}

			if got := body.Services[serviceDatabase]; got != tt.wantDBStatus {
				t.Errorf("services[database] = %q, want %q", got, tt.wantDBStatus)
			}
		})
	}
}
