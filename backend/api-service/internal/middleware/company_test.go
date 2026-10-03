package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/identity"
)

type companyValidator struct {
	role   string
	active bool
	err    error
}

func (v companyValidator) ValidateAccessToken(_ context.Context, token string) (string, error) {
	if token != "valid-token" {
		return "", errors.New("invalid token")
	}

	return "user-1", nil
}

func (v companyValidator) Membership(_ context.Context, userID, companyID string) (string, bool, error) {
	if userID != "user-1" || companyID != "company-1" {
		return "", false, nil
	}

	return v.role, v.active, v.err
}

func TestRequireCompanyPrincipal(t *testing.T) {
	tests := []struct {
		name       string
		companyID  string
		token      string
		validator  companyValidator
		wantStatus int
	}{
		{"active membership", "company-1", "valid-token", companyValidator{role: "employee", active: true}, http.StatusNoContent},
		{"no token", "company-1", "", companyValidator{role: "employee", active: true}, http.StatusUnauthorized},
		{"invalid token", "company-1", "bad-token", companyValidator{role: "employee", active: true}, http.StatusUnauthorized},
		{"other company", "company-2", "valid-token", companyValidator{role: "employee", active: true}, http.StatusNotFound},
		{"inactive membership", "company-1", "valid-token", companyValidator{role: "employee"}, http.StatusNotFound},
		{"lookup failure", "company-1", "valid-token", companyValidator{err: errors.New("database unavailable")}, http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := http.NewServeMux()
			router.Handle("GET /companies/{cid}/modules", RequireCompanyPrincipal(tt.validator)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				principal, ok := identity.FromContext(r.Context())
				if !ok || principal.UserID != "user-1" || principal.CompanyID != "company-1" || principal.Role != "employee" {
					t.Errorf("invalid principal: %+v, ok=%v", principal, ok)
				}

				w.WriteHeader(http.StatusNoContent)
			})))

			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/companies/"+tt.companyID+"/modules", nil)
			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
