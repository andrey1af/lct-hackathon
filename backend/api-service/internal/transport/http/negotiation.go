package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/identity"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/negotiation"
)

type NegotiationHandler struct{ service *negotiation.Service }

func NewNegotiationHandler(service *negotiation.Service) *NegotiationHandler {
	return &NegotiationHandler{service: service}
}

func (h *NegotiationHandler) ListCases(w http.ResponseWriter, r *http.Request) {
	companyID, userID := negotiationActor(r)

	cases, err := h.service.ListCases(r.Context(), companyID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not load cases")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"cases": cases})
}

func (h *NegotiationHandler) GetCase(w http.ResponseWriter, r *http.Request) {
	companyID, userID := negotiationActor(r)
	caseView, err := h.service.GetCase(r.Context(), companyID, userID, r.PathValue("id"))
	if errors.Is(err, negotiation.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "Case not found")

		return
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not load case")

		return
	}

	writeJSON(w, http.StatusOK, caseView)
}

func (h *NegotiationHandler) StartSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CaseKey string `json:"case_key"`
	}

	decoder := json.NewDecoder(io.LimitReader(r.Body, 8192))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")

		return
	}

	companyID, userID := negotiationActor(r)

	session, err := h.service.StartSession(r.Context(), companyID, userID, body.CaseKey)
	writeSession(w, session, err)
}

func (h *NegotiationHandler) StartAssessment(w http.ResponseWriter, r *http.Request) {
	companyID, userID := negotiationActor(r)
	session, err := h.service.StartAssessment(r.Context(), companyID, userID, r.PathValue("id"))
	writeSession(w, session, err)
}

func (h *NegotiationHandler) RetrySession(w http.ResponseWriter, r *http.Request) {
	companyID, userID := negotiationActor(r)

	session, err := h.service.RetrySession(r.Context(), companyID, userID, r.PathValue("id"))
	writeSession(w, session, err)
}

func writeSession(w http.ResponseWriter, session negotiation.SessionView, err error) {
	switch {
	case errors.Is(err, negotiation.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "Active membership required")

	case errors.Is(err, negotiation.ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid_request", "Case cannot be started as a standalone dialogue")

	case errors.Is(err, negotiation.ErrLocked):
		writeError(w, http.StatusConflict, "assessment_locked", "Finish the module theory before starting the assessment")

	case errors.Is(err, negotiation.ErrConflict):
		writeError(w, http.StatusConflict, "invalid_session_state", "Finish the current attempt before starting a new one")

	case errors.Is(err, negotiation.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Scenario is not published")

	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not start session")

	default:
		writeJSON(w, http.StatusCreated, session)
	}
}

func (h *NegotiationHandler) Hint(w http.ResponseWriter, r *http.Request) {
	companyID, userID := negotiationActor(r)

	hint, err := h.service.Hint(r.Context(), companyID, userID, r.PathValue("id"))

	switch {
	case errors.Is(err, negotiation.ErrForbidden):
		writeError(w, http.StatusForbidden, "hints_disabled", "Hints are not available in assessment cases")

	case errors.Is(err, negotiation.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Session not found")

	case errors.Is(err, negotiation.ErrConflict):
		writeError(w, http.StatusConflict, "hint_unavailable", "Session is not active or the hint limit is reached")

	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not prepare a hint")

	default:
		writeJSON(w, http.StatusOK, hint)
	}
}

func (h *NegotiationHandler) FinishSession(w http.ResponseWriter, r *http.Request) {
	companyID, userID := negotiationActor(r)

	result, err := h.service.FinishSession(r.Context(), companyID, userID, r.PathValue("id"))
	writeResult(w, result, err)
}

func (h *NegotiationHandler) GetResult(w http.ResponseWriter, r *http.Request) {
	companyID, userID := negotiationActor(r)

	result, err := h.service.GetResult(r.Context(), companyID, userID, r.PathValue("id"))
	writeResult(w, result, err)
}

func writeResult(w http.ResponseWriter, result negotiation.ResultView, err error) {
	switch {
	case errors.Is(err, negotiation.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Session not found")

	case errors.Is(err, negotiation.ErrConflict):
		writeError(w, http.StatusConflict, "session_not_evaluated", "Session is still active")

	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not evaluate session")

	default:
		writeJSON(w, http.StatusOK, result)
	}
}

func (h *NegotiationHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ClientMessageID string `json:"client_message_id"`
		Text            string `json:"text"`
	}

	decoder := json.NewDecoder(io.LimitReader(r.Body, 8192))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")

		return
	}

	companyID, userID := negotiationActor(r)

	turn, err := h.service.SendMessage(r.Context(), companyID, userID, r.PathValue("id"), body.ClientMessageID, body.Text)

	switch {
	case errors.Is(err, negotiation.ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid_request", "Message text must not be empty")

	case errors.Is(err, negotiation.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Session not found")

	case errors.Is(err, negotiation.ErrConflict):
		writeError(w, http.StatusConflict, "invalid_session_state", "Session is not active")

	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not process message")

	default:
		writeJSON(w, http.StatusOK, turn)
	}
}

func (h *NegotiationHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	companyID, userID := negotiationActor(r)

	session, err := h.service.GetSession(r.Context(), companyID, userID, r.PathValue("id"))

	switch {
	case errors.Is(err, negotiation.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Session not found")

	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not load session")

	default:
		writeJSON(w, http.StatusOK, session)
	}
}

func negotiationActor(r *http.Request) (companyID, userID string) {
	p, ok := identity.FromContext(r.Context())
	if !ok {
		return "", ""
	}

	return p.CompanyID, p.UserID
}
