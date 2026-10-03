package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/config"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/content"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/database"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/middleware"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/model"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/repository"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/services/auth"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/services/learning"
)

func TestPlatformInvitationFlow(t *testing.T) {
	baseDSN := os.Getenv("BE3_TEST_DSN")
	if baseDSN == "" {
		t.Skip("set BE3_TEST_DSN to a disposable PostgreSQL database")
	}

	adminDB, err := gorm.Open(postgres.Open(baseDSN), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	schema := fmt.Sprintf("platform_test_%d", time.Now().UnixNano())
	if err := adminDB.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = adminDB.Exec("DROP SCHEMA " + schema + " CASCADE").Error })
	u, err := url.Parse(baseDSN)
	if err != nil {
		t.Fatal(err)
	}

	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	db, err := gorm.Open(postgres.Open(u.String()), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	data, err := content.Load("../../../content")
	if err != nil {
		t.Fatal(err)
	}

	if err := content.Seed(context.Background(), db, data); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{SETTINGS: config.SettingsConfig{JWTSecret: "integration-test-secret", AccessTokenTTL: time.Hour, RefreshTokenTTL: time.Hour}}
	logger := slog.Default()
	service := auth.NewAuthService(
		repository.NewUserRepo(*db, logger),
		repository.NewTokenRepo(*db, logger),
		cfg,
		db,
		logger,
	)
	companyID, err := service.BootstrapAdmin(context.Background(), "Acme", "admin@example.test", "Admin", "strong-password")
	if err != nil {
		t.Fatal(err)
	}

	adminTokens, err := service.Login(context.Background(), auth.LoginRequest{Email: "admin@example.test", Password: "strong-password"})
	if err != nil {
		t.Fatal(err)
	}

	router := NewRouter(NewHealthHandler(fakePinger{}, time.Second), nil, nil, NewLearningHandler(learning.NewService(db, slog.Default())), nil,
		middleware.RequireAccessToken(service), middleware.RequireCompanyPrincipal(service), NewUserHandler(service), NewPlatformHandler(service))
	loginReq := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"admin@example.test","password":"strong-password"}`))
	loginRec := httptest.NewRecorder()
	router.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK || strings.Contains(loginRec.Body.String(), "refreshToken") {
		t.Fatalf("cookie login: %d %s", loginRec.Code, loginRec.Body.String())
	}

	var accessCookie, refreshCookie *http.Cookie

	for _, cookie := range loginRec.Result().Cookies() {
		switch cookie.Name {
		case "access_token":
			accessCookie = cookie

		case "refresh_token":
			refreshCookie = cookie
		}
	}

	if accessCookie == nil || refreshCookie == nil || !accessCookie.HttpOnly || !refreshCookie.HttpOnly || !accessCookie.Secure || !refreshCookie.Secure {
		t.Fatal("secure auth cookies were not set")
	}

	meReq := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/me", nil)
	meReq.AddCookie(accessCookie)
	meRec := httptest.NewRecorder()
	router.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("cookie auth: %d %s", meRec.Code, meRec.Body.String())
	}

	refreshReq := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/auth/refresh", nil)
	refreshReq.AddCookie(refreshCookie)
	refreshRec := httptest.NewRecorder()
	router.ServeHTTP(refreshRec, refreshReq)
	if refreshRec.Code != http.StatusOK {
		t.Fatalf("cookie refresh: %d %s", refreshRec.Code, refreshRec.Body.String())
	}

	reused := httptest.NewRecorder()
	router.ServeHTTP(reused, refreshReq)
	if reused.Code != http.StatusUnauthorized {
		t.Fatalf("reused refresh token: %d", reused.Code)
	}

	request := func(method, path, body, token string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		return rec
	}
	invitePath := "/api/v1/companies/" + companyID + "/invitations"
	invite := request(http.MethodPost, invitePath, `{"email":"employee@example.test","role":"employee"}`, adminTokens.AccessToken)
	if invite.Code != http.StatusCreated {
		t.Fatalf("invite: %d %s", invite.Code, invite.Body.String())
	}

	var invitation auth.InvitationView

	if err := json.Unmarshal(invite.Body.Bytes(), &invitation); err != nil {
		t.Fatal(err)
	}

	acceptBody, _ := json.Marshal(map[string]string{"token": invitation.Token, "name": "Employee", "password": "strong-password"})
	accept := request(http.MethodPost, "/api/v1/invitations/accept", string(acceptBody), "")
	if accept.Code != http.StatusOK {
		t.Fatalf("accept: %d %s", accept.Code, accept.Body.String())
	}

	if got := request(http.MethodPost, "/api/v1/invitations/accept", string(acceptBody), "").Code; got != http.StatusConflict {
		t.Fatalf("reused invitation: %d", got)
	}

	employeeTokens, err := service.Login(context.Background(), auth.LoginRequest{Email: "employee@example.test", Password: "strong-password"})
	if err != nil {
		t.Fatal(err)
	}

	if got := request(http.MethodGet, "/api/v1/me", "", employeeTokens.AccessToken); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), companyID) {
		t.Fatalf("profile: %d %s", got.Code, got.Body.String())
	}

	if got := request(http.MethodGet, "/api/v1/companies/"+companyID+"/modules", "", employeeTokens.AccessToken).Code; got != http.StatusOK {
		t.Fatalf("employee modules: %d", got)
	}

	if got := request(http.MethodGet, "/api/v1/companies/"+companyID+"/team/progress", "", employeeTokens.AccessToken).Code; got != http.StatusForbidden {
		t.Fatalf("employee team progress: %d", got)
	}

	if got := request(http.MethodGet, "/api/v1/companies/"+companyID+"/members", "", adminTokens.AccessToken).Code; got != http.StatusOK {
		t.Fatalf("admin members: %d", got)
	}

	if got := request(http.MethodGet, "/api/v1/companies/"+companyID+"/team/progress", "", adminTokens.AccessToken).Code; got != http.StatusOK {
		t.Fatalf("admin team progress: %d", got)
	}

	var admin model.User

	if err := db.Where("email = ?", "admin@example.test").Take(&admin).Error; err != nil {
		t.Fatal(err)
	}

	if got := request(http.MethodPatch, "/api/v1/companies/"+companyID+"/members/"+admin.ID, `{"role":"employee","status":"active"}`, adminTokens.AccessToken).Code; got != http.StatusConflict {
		t.Fatalf("last admin demotion: %d", got)
	}

	otherCompany := model.Company{ID: "83000000-0000-4000-8000-000000009999", Name: "Other"}
	if err := db.Create(&otherCompany).Error; err != nil {
		t.Fatal(err)
	}

	if got := request(http.MethodGet, "/api/v1/companies/"+otherCompany.ID+"/modules", "", adminTokens.AccessToken).Code; got != http.StatusNotFound {
		t.Fatalf("cross-company access: %d", got)
	}

	if got := request(http.MethodGet, "/api/v1/companies/"+otherCompany.ID+"/team/progress", "", adminTokens.AccessToken).Code; got != http.StatusNotFound {
		t.Fatalf("cross-company team progress: %d", got)
	}

	if err := db.Create(&model.Membership{CompanyID: otherCompany.ID, UserID: admin.ID, Role: "company_admin", Status: "active"}).Error; err != nil {
		t.Fatal(err)
	}

	secondInvite := request(http.MethodPost, "/api/v1/companies/"+otherCompany.ID+"/invitations", `{"email":"employee@example.test","role":"employee"}`, adminTokens.AccessToken)
	if secondInvite.Code != http.StatusCreated {
		t.Fatalf("second company invite: %d %s", secondInvite.Code, secondInvite.Body.String())
	}

	if err := json.Unmarshal(secondInvite.Body.Bytes(), &invitation); err != nil {
		t.Fatal(err)
	}

	joinBody, _ := json.Marshal(map[string]string{"token": invitation.Token})
	joinReq := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/invitations/accept", strings.NewReader(string(joinBody)))
	joinReq.AddCookie(&http.Cookie{Name: "access_token", Value: employeeTokens.AccessToken})
	joinRec := httptest.NewRecorder()
	router.ServeHTTP(joinRec, joinReq)
	if joinRec.Code != http.StatusOK || !strings.Contains(joinRec.Body.String(), otherCompany.ID) {
		t.Fatalf("existing user joins second company: %d %s", joinRec.Code, joinRec.Body.String())
	}
}
