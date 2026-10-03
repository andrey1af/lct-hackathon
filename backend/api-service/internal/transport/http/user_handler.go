package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/middleware"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/model"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/services/auth"
)

const maxBodyBytes = 1 << 20

type UserHandler struct {
	service *auth.AuthService
}

func NewUserHandler(service *auth.AuthService) *UserHandler {
	return &UserHandler{service: service}
}

func (h *UserHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req auth.RegisterRequest

	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", err.Error())

		return
	}

	if !isValidEmail(req.Email) {
		writeError(w, http.StatusBadRequest, "invalid email", "Invalid email")

		return
	}

	tokens, err := h.service.Register(r.Context(), req)
	if err != nil {
		h.fail(w, r, err)

		return
	}

	h.setAuthCookies(w, tokens)
	writeJSON(w, http.StatusCreated, auth.ToJWTResponse(tokens))
}

func (h *UserHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req auth.LoginRequest

	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", err.Error())

		return
	}

	if !isValidEmail(req.Email) {
		writeError(w, http.StatusBadRequest, "invalid email", "Invalid email")

		return
	}

	tokens, err := h.service.Login(r.Context(), req)
	if err != nil {
		h.fail(w, r, err)

		return
	}

	h.setAuthCookies(w, tokens)
	writeJSON(w, http.StatusOK, auth.ToJWTResponse(tokens))
}

func (h *UserHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req auth.RefreshRequest

	if cookie, err := r.Cookie(h.refreshCookieName()); err == nil {
		req.RefreshToken = cookie.Value
	} else if err := decodeJSON(w, r, &req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")

		return
	}

	if strings.TrimSpace(req.RefreshToken) == "" {
		writeError(w, http.StatusUnauthorized, "invalid refresh token", "Invalid refresh token")

		return
	}

	tokens, err := h.service.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		if errors.Is(err, model.ErrInvalidToken) {
			writeError(w, http.StatusUnauthorized, "invalid refresh token for refresh", "Invalid refresh token")

			return
		}

		writeError(w, http.StatusInternalServerError, "internal_error", "Could not refresh session")

		return
	}

	h.setAuthCookies(w, tokens)
	writeJSON(w, http.StatusOK, auth.ToJWTResponse(tokens))
}

func (h *UserHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req auth.LogoutRequest

	if cookie, err := r.Cookie(h.refreshCookieName()); err == nil {
		req.RefreshToken = cookie.Value
	} else if err := decodeJSON(w, r, &req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusUnprocessableEntity, "invalid refresh token for logout", "Invalid refresh token")

		return
	}

	req.RefreshToken = strings.TrimSpace(req.RefreshToken)
	if req.RefreshToken == "" {
		writeError(w, http.StatusUnprocessableEntity, "invalid refresh token for logout", "Invalid refresh token")

		return
	}

	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Unauthorized")

		return
	}

	err := h.service.Logout(r.Context(), userID, req.RefreshToken)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", "Failed to logout user")

		return
	}

	h.clearAuthCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *UserHandler) refreshCookieName() string {
	name, _, _, _ := h.service.CookieSettings()

	return name
}

func (h *UserHandler) setAuthCookies(w http.ResponseWriter, tokens *model.JWT) {
	name, accessTTL, refreshTTL, secure := h.service.CookieSettings()

	for _, item := range []struct {
		name, value string
		ttl         time.Duration
	}{{"access_token", tokens.AccessToken, accessTTL}, {name, tokens.RefreshToken, refreshTTL}} {
		http.SetCookie(w, &http.Cookie{
			Name: item.name, Value: item.value, Path: "/", HttpOnly: true,
			Secure: secure, SameSite: http.SameSiteLaxMode, Expires: time.Now().Add(item.ttl),
		})
	}
}

func (h *UserHandler) clearAuthCookies(w http.ResponseWriter) {
	name, _, _, secure := h.service.CookieSettings()

	for _, cookieName := range []string{"access_token", name} {
		http.SetCookie(w, &http.Cookie{
			Name: cookieName, Path: "/", HttpOnly: true, Secure: secure,
			SameSite: http.SameSiteLaxMode, MaxAge: -1,
		})
	}
}

func isValidEmail(s string) bool {
	addr, err := mail.ParseAddress(s)

	return err == nil && addr.Address == s
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	return json.NewDecoder(r.Body).Decode(dst)
}

func (h *UserHandler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, model.ErrEmailTaken):
		writeError(w, http.StatusConflict, "email already registered", "Email already registered")

	case errors.Is(err, model.ErrWeakPassword):
		writeError(w, http.StatusBadRequest, "password must be 8-72 bytes long", "Password must be 8-72 bytes long")

	case errors.Is(err, model.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid input", "Invalid input")

	case errors.Is(err, model.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, "invalid email or password", "Invalid email or password")

	case errors.Is(err, model.ErrInvalidToken):
		writeError(w, http.StatusUnauthorized, "unauthorized", "Unauthorized")

	default:
		writeError(w, http.StatusInternalServerError, "internal error", err.Error())
	}
}
