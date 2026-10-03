package http

import (
	jsonv2 "encoding/json/v2"
	"net/http"
)

func NewRouter(
	health *HealthHandler,
	corsAllowedOrigins []string,
	openAPISpec []byte,
	learning *LearningHandler,
	negotiation *NegotiationHandler,
	authMiddleware func(http.Handler) http.Handler,
	companyMiddleware func(http.Handler) http.Handler,
	userHandler *UserHandler,
	platform *PlatformHandler,
) http.Handler {
	router := http.NewServeMux()

	router.HandleFunc("GET /health/live", health.Liveness)
	router.HandleFunc("GET /health/ready", health.Readiness)

	router.HandleFunc("GET /openapi.yaml", newOpenAPISpecHandler(openAPISpec))
	router.HandleFunc("GET /swagger", swaggerUIHandler)
	router.HandleFunc("GET /swagger/", swaggerUIHandler)
	if learning != nil {
		router.Handle("GET /api/v1/companies/{cid}/me/progress", companyMiddleware(http.HandlerFunc(learning.MeProgress)))
		router.Handle("GET /api/v1/companies/{cid}/team/progress", companyMiddleware(http.HandlerFunc(learning.TeamProgress)))
		router.Handle("GET /api/v1/companies/{cid}/modules", companyMiddleware(http.HandlerFunc(learning.ListModules)))
		router.Handle("GET /api/v1/companies/{cid}/modules/{id}", companyMiddleware(http.HandlerFunc(learning.GetModule)))
		router.Handle("POST /api/v1/companies/{cid}/steps/{id}/attempts", companyMiddleware(http.HandlerFunc(learning.SubmitAttempt)))
	}

	// auth
	router.Handle("POST /auth/register", http.HandlerFunc(userHandler.Register))
	login := loginRateLimit(http.HandlerFunc(userHandler.Login))
	router.Handle("POST /api/v1/auth/login", login)
	router.Handle("POST /api/v1/auth/logout", authMiddleware(http.HandlerFunc(userHandler.Logout)))
	router.Handle("POST /api/v1/auth/refresh", http.HandlerFunc(userHandler.Refresh))
	if platform != nil {
		router.Handle("GET /api/v1/me", authMiddleware(http.HandlerFunc(platform.Me)))
		router.Handle("PATCH /api/v1/me", authMiddleware(http.HandlerFunc(platform.UpdateMe)))
		router.Handle("POST /api/v1/invitations/accept", http.HandlerFunc(platform.AcceptInvite))
		router.Handle("GET /api/v1/companies/{cid}/members", companyMiddleware(http.HandlerFunc(platform.Members)))
		router.Handle("PATCH /api/v1/companies/{cid}/members/{uid}", companyMiddleware(http.HandlerFunc(platform.UpdateMember)))
		router.Handle("POST /api/v1/companies/{cid}/invitations", companyMiddleware(http.HandlerFunc(platform.Invite)))
		router.Handle("GET /api/v1/companies/{cid}/me/sessions", companyMiddleware(http.HandlerFunc(platform.Sessions)))
	}

	// negotiation
	if negotiation != nil {
		router.Handle("POST /api/v1/companies/{cid}/modules/{id}/assessment-sessions", companyMiddleware(http.HandlerFunc(negotiation.StartAssessment)))
		router.Handle("GET /api/v1/companies/{cid}/cases", companyMiddleware(http.HandlerFunc(negotiation.ListCases)))
		router.Handle("GET /api/v1/companies/{cid}/cases/{id}", companyMiddleware(http.HandlerFunc(negotiation.GetCase)))
		router.Handle("POST /api/v1/companies/{cid}/sessions", companyMiddleware(http.HandlerFunc(negotiation.StartSession)))
		router.Handle("GET /api/v1/companies/{cid}/sessions/{id}", companyMiddleware(http.HandlerFunc(negotiation.GetSession)))
		router.Handle("POST /api/v1/companies/{cid}/sessions/{id}/messages", companyMiddleware(http.HandlerFunc(negotiation.SendMessage)))
		router.Handle("POST /api/v1/companies/{cid}/sessions/{id}/hint", companyMiddleware(http.HandlerFunc(negotiation.Hint)))
		router.Handle("POST /api/v1/companies/{cid}/sessions/{id}/retry", companyMiddleware(http.HandlerFunc(negotiation.RetrySession)))
		router.Handle("POST /api/v1/companies/{cid}/sessions/{id}/finish", companyMiddleware(http.HandlerFunc(negotiation.FinishSession)))
		router.Handle("GET /api/v1/companies/{cid}/sessions/{id}/feedback", companyMiddleware(http.HandlerFunc(negotiation.GetResult)))
	}

	return CORS(corsAllowedOrigins)(router)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = jsonv2.MarshalWrite(w, value)
}
