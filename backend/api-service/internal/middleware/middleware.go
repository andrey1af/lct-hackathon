package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type errorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	resp := errorResponse{}
	resp.Error.Code = code
	resp.Error.Message = message

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}

type AccessTokenValidator interface {
	ValidateAccessToken(ctx context.Context, token string) (userID string, err error)
}

type contextKey struct{}

func RequireAccessToken(validator AccessTokenValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				if cookie, err := r.Cookie("access_token"); err == nil && cookie.Value != "" {
					token, ok = cookie.Value, true
				}
			}

			if !ok {
				writeError(w, http.StatusUnauthorized, "missing or invalid authorization header", "Missing or invalid authorization header")

				return
			}

			userID, err := validator.ValidateAccessToken(r.Context(), token)
			if err != nil || userID == "" {
				writeError(w, http.StatusUnauthorized, "unauthorized", "Unauthorized")

				return
			}

			ctx := context.WithValue(r.Context(), contextKey{}, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func UserIDFromContext(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(contextKey{}).(string)

	return userID, ok && userID != ""
}

func bearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}

	return parts[1], true
}
