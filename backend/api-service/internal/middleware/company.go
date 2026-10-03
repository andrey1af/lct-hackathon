package middleware

import (
	"context"
	"net/http"

	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/identity"
)

type CompanyPrincipalValidator interface {
	AccessTokenValidator
	Membership(ctx context.Context, userID, companyID string) (role string, active bool, err error)
}

func RequireCompanyPrincipal(validator CompanyPrincipalValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return RequireAccessToken(validator)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := UserIDFromContext(r.Context())
			if !ok {
				writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")

				return
			}

			companyID := r.PathValue("cid")
			if companyID == "" {
				writeError(w, http.StatusNotFound, "not_found", "Company not found")

				return
			}

			role, active, err := validator.Membership(r.Context(), userID, companyID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", "Could not check membership")

				return
			}

			if !active {
				writeError(w, http.StatusNotFound, "not_found", "Company not found")

				return
			}

			principal := identity.Principal{UserID: userID, CompanyID: companyID, Role: role, Active: true}
			next.ServeHTTP(w, r.WithContext(identity.WithPrincipal(r.Context(), principal)))
		}))
	}
}
