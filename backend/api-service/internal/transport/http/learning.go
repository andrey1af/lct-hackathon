package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/identity"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/services/learning"
)

type LearningHandler struct{ service *learning.Service }

func NewLearningHandler(service *learning.Service) *LearningHandler {
	return &LearningHandler{service: service}
}

func (h *LearningHandler) MeProgress(w http.ResponseWriter, r *http.Request) {
	p, ok := learningPrincipal(w, r)
	if !ok {
		return
	}

	modules, err := h.service.ListModules(r.Context(), p.CompanyID, p.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not load progress")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"modules": modules})
}

func (h *LearningHandler) TeamProgress(w http.ResponseWriter, r *http.Request) {
	p, ok := learningPrincipal(w, r)
	if !ok {
		return
	}

	if p.Role != "company_admin" && p.Role != "platform_admin" {
		writeError(w, http.StatusForbidden, "forbidden", "Company admin required")

		return
	}

	members, err := h.service.TeamProgress(r.Context(), p.CompanyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not load team progress")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"members": members})
}

type apiError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	response := apiError{}
	response.Error.Code = code
	response.Error.Message = message
	writeJSON(w, status, response)
}

func learningPrincipal(w http.ResponseWriter, r *http.Request) (identity.Principal, bool) {
	p, ok := identity.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")

		return identity.Principal{}, false
	}

	if p.CompanyID != r.PathValue("cid") {
		writeError(w, http.StatusNotFound, "not_found", "Resource not found")

		return identity.Principal{}, false
	}

	if p.Role != "employee" && p.Role != "company_admin" && p.Role != "platform_admin" {
		writeError(w, http.StatusForbidden, "forbidden", "Active membership required")

		return identity.Principal{}, false
	}

	return p, true
}

func (h *LearningHandler) ListModules(w http.ResponseWriter, r *http.Request) {
	p, ok := learningPrincipal(w, r)
	if !ok {
		return
	}

	modules, err := h.service.ListModules(r.Context(), p.CompanyID, p.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not load modules")

		return
	}

	writeJSON(w, http.StatusOK, struct {
		Modules []learning.ModuleView `json:"modules"`
	}{Modules: modules})
}

func (h *LearningHandler) GetModule(w http.ResponseWriter, r *http.Request) {
	p, ok := learningPrincipal(w, r)
	if !ok {
		return
	}

	module, err := h.service.GetModule(r.Context(), p.CompanyID, p.UserID, r.PathValue("id"))
	if errors.Is(err, learning.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "Resource not found")

		return
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not load module")

		return
	}

	writeJSON(w, http.StatusOK, module)
}

func (h *LearningHandler) SubmitAttempt(w http.ResponseWriter, r *http.Request) {
	p, ok := learningPrincipal(w, r)
	if !ok {
		return
	}

	var body struct {
		ClientAttemptID string `json:"client_attempt_id"`
		Answer          string `json:"answer"`
	}

	decoder := json.NewDecoder(io.LimitReader(r.Body, 8192))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")

		return
	}

	var trailing any

	if err := decoder.Decode(&trailing); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid_request", "Expected a single JSON object")

		return
	}

	result, err := h.service.SubmitAttempt(r.Context(), p.CompanyID, p.UserID, r.PathValue("id"), body.ClientAttemptID, body.Answer)

	switch {
	case errors.Is(err, learning.ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid_answer", "Invalid attempt answer or key")

	case errors.Is(err, learning.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Resource not found")

	case errors.Is(err, learning.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "Step is closed or attempt key conflicts")

	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not save attempt")

	default:
		writeJSON(w, http.StatusOK, result)
	}
}
