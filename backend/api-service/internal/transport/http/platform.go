package http

import (
	"errors"
	"net/http"
	"strings"

	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/identity"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/middleware"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/model"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/services/auth"
)

type PlatformHandler struct{ service *auth.AuthService }

func NewPlatformHandler(service *auth.AuthService) *PlatformHandler {
	return &PlatformHandler{service: service}
}

func platformError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, model.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid input")

	case errors.Is(err, model.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, "unauthorized", "Login with the invited email")

	case errors.Is(err, auth.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "Company admin required")

	case errors.Is(err, auth.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Resource not found")

	case errors.Is(err, auth.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "Invitation or membership cannot be changed")

	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Request could not be completed")
	}
}

func (h *PlatformHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")

		return
	}

	profile, err := h.service.Profile(r.Context(), userID)
	if err != nil {
		platformError(w, err)

		return
	}

	writeJSON(w, http.StatusOK, profile)
}

func (h *PlatformHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")

		return
	}

	var body struct {
		Name string `json:"name"`
	}

	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")

		return
	}

	profile, err := h.service.UpdateProfile(r.Context(), userID, body.Name)
	if err != nil {
		platformError(w, err)

		return
	}

	writeJSON(w, http.StatusOK, profile)
}

func companyPrincipal(w http.ResponseWriter, r *http.Request) (identity.Principal, bool) {
	p, ok := identity.FromContext(r.Context())
	if !ok || p.CompanyID != r.PathValue("cid") {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")

		return identity.Principal{}, false
	}

	return p, true
}

func (h *PlatformHandler) Members(w http.ResponseWriter, r *http.Request) {
	p, ok := companyPrincipal(w, r)
	if !ok {
		return
	}

	members, err := h.service.Members(r.Context(), p.CompanyID, p.UserID)
	if err != nil {
		platformError(w, err)

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"members": members})
}

func (h *PlatformHandler) UpdateMember(w http.ResponseWriter, r *http.Request) {
	p, ok := companyPrincipal(w, r)
	if !ok {
		return
	}

	var body struct {
		Role   string `json:"role"`
		Status string `json:"status"`
	}

	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")

		return
	}

	if err := h.service.UpdateMember(r.Context(), p.CompanyID, p.UserID, r.PathValue("uid"), body.Role, body.Status); err != nil {
		platformError(w, err)

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *PlatformHandler) Invite(w http.ResponseWriter, r *http.Request) {
	p, ok := companyPrincipal(w, r)
	if !ok {
		return
	}

	var body struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}

	if err := decodeJSON(w, r, &body); err != nil || !isValidEmail(body.Email) {
		writeError(w, http.StatusBadRequest, "invalid_request", "Valid email and role required")

		return
	}

	invitation, err := h.service.CreateInvitation(r.Context(), p.CompanyID, p.UserID, body.Email, body.Role)
	if err != nil {
		platformError(w, err)

		return
	}

	writeJSON(w, http.StatusCreated, invitation)
}

func (h *PlatformHandler) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token    string `json:"token"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}

	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")

		return
	}

	userID := ""
	token := ""
	if header := strings.Fields(r.Header.Get("Authorization")); len(header) == 2 && strings.EqualFold(header[0], "Bearer") {
		token = header[1]
	} else if cookie, err := r.Cookie("access_token"); err == nil {
		token = cookie.Value
	}

	if token != "" {
		var err error

		userID, err = h.service.ValidateAccessToken(r.Context(), token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid access token")

			return
		}
	}

	profile, err := h.service.AcceptInvitation(r.Context(), body.Token, userID, body.Name, body.Password)
	if err != nil {
		platformError(w, err)

		return
	}

	writeJSON(w, http.StatusOK, profile)
}

func (h *PlatformHandler) Sessions(w http.ResponseWriter, r *http.Request) {
	p, ok := companyPrincipal(w, r)
	if !ok {
		return
	}

	sessions, err := h.service.Sessions(r.Context(), p.CompanyID, p.UserID)
	if err != nil {
		platformError(w, err)

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}
