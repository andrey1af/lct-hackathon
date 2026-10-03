package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/content"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/database"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/identity"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/llm"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/model"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/negotiation"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/services/learning"
)

func TestLearningPostgresFlow(t *testing.T) {
	baseDSN := os.Getenv("BE3_TEST_DSN")
	if baseDSN == "" {
		t.Skip("set BE3_TEST_DSN to a disposable PostgreSQL database")
	}

	u, err := url.Parse(baseDSN)
	if err != nil {
		t.Fatal(err)
	}

	adminDB, err := gorm.Open(postgres.Open(baseDSN), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	schema := fmt.Sprintf("be3_test_%d", time.Now().UnixNano())
	if err := adminDB.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = adminDB.Exec("DROP SCHEMA " + schema + " CASCADE").Error })
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := gorm.Open(postgres.Open(u.String()), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatalf("repeat migration: %v", err)
	}

	if !db.Migrator().HasTable("refresh_tokens") {
		t.Fatal("refresh_tokens migration was not applied")
	}

	data, err := content.Load("../../../content")
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		if err := content.Seed(context.Background(), db, data); err != nil {
			t.Fatal(err)
		}
	}

	wantSteps := 0

	for _, item := range data.Modules {
		wantSteps += len(item.Steps)
	}

	wantRows := map[string]int64{
		"modules":         int64(len(data.Modules)),
		"module_versions": int64(len(data.Modules)),
		"steps":           int64(wantSteps),
		"cases":           int64(len(data.Cases)),
		"case_versions":   int64(len(data.Cases)),
	}

	for table, want := range wantRows {
		var got int64

		if err := db.Table(table).Count(&got).Error; err != nil || got != want {
			t.Fatalf("%s count=%d, want=%d, err=%v", table, got, want, err)
		}
	}

	if err := db.Exec(`UPDATE module_versions SET title = 'changed' WHERE id = ?`, "83000000-0000-4000-8000-000000000111").Error; err == nil {
		t.Fatal("database allowed mutation of published module version")
	}

	changed := data
	changed.Cases = append([]content.CaseFile(nil), data.Cases...)
	changed.Cases[0].Hash = strings.Repeat("0", 64)
	if err := content.Seed(context.Background(), db, changed); err == nil {
		t.Fatal("changed published version was accepted")
	}

	companyA := "83000000-0000-4000-8000-000000000301"
	companyB := "83000000-0000-4000-8000-000000000302"
	userA := "83000000-0000-4000-8000-000000000311"
	userB := "83000000-0000-4000-8000-000000000312"

	for _, row := range []model.Company{{ID: companyA, Name: "A"}, {ID: companyB, Name: "B"}} {
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}

	for _, row := range []model.User{{ID: userA, Email: "a@example.test", Name: "A", PasswordHash: "test"}, {ID: userB, Email: "b@example.test", Name: "B", PasswordHash: "test"}} {
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}

	for _, row := range []model.Membership{{CompanyID: companyA, UserID: userA, Role: "employee", Status: "active"}, {CompanyID: companyA, UserID: userB, Role: "employee", Status: "active"}, {CompanyID: companyB, UserID: userA, Role: "employee", Status: "active"}} {
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}

	badAttempt := model.LearningAttempt{ID: "83000000-0000-4000-8000-000000000350", CompanyID: companyA, UserID: userA, StepID: "83000000-0000-4000-8000-000000000121", ModuleVersionID: "83000000-0000-4000-8000-000000000112", ClientAttemptID: "invalid-pair", Answer: "confirmed", Correct: true, Completed: true, ResultJSON: json.RawMessage(`{}`), CreatedAt: time.Now()}
	if err := db.Create(&badAttempt).Error; err == nil {
		t.Fatal("database accepted step with wrong module version")
	}

	session := model.Session{ID: "83000000-0000-4000-8000-000000000360", CompanyID: companyA, UserID: userA, CaseVersionID: "83000000-0000-4000-8000-000000000213", Mode: "practice", Status: "active", StateJSON: json.RawMessage(`{}`), Version: 1, CreatedAt: time.Now()}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}

	messageKey := "same-key"
	message := model.Message{ID: "83000000-0000-4000-8000-000000000361", SessionID: session.ID, ClientMessageID: &messageKey, TurnNumber: 1, Role: "user", Text: "Первый ход", CreatedAt: time.Now()}
	if err := db.Create(&message).Error; err != nil {
		t.Fatal(err)
	}

	message.ID = "83000000-0000-4000-8000-000000000362"
	message.TurnNumber = 2
	if err := db.Create(&message).Error; err == nil {
		t.Fatal("database accepted duplicate client_message_id in session")
	}

	principal := identity.Principal{UserID: userA, CompanyID: companyA, Role: "employee", Active: true}
	pass := func(next http.Handler) http.Handler { return next }
	router := NewRouter(NewHealthHandler(fakePinger{}, time.Second), []string{"*"}, nil, NewLearningHandler(learning.NewService(db, slog.Default())), nil, pass, pass, NewUserHandler(nil), nil)
	request := func(method, path, body string, p *identity.Principal) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
		if p != nil {
			req = req.WithContext(identity.WithPrincipal(req.Context(), *p))
		}

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		return rec
	}

	listPath := "/api/v1/companies/" + companyA + "/modules"
	if got := request(http.MethodGet, listPath, "", nil).Code; got != 401 {
		t.Fatalf("unauthenticated status=%d", got)
	}

	if got := request(http.MethodGet, "/api/v1/companies/"+companyB+"/modules", "", &principal).Code; got != 404 {
		t.Fatalf("cross-company status=%d", got)
	}

	list := request(http.MethodGet, listPath, "", &principal)
	if list.Code != 200 || !strings.Contains(list.Body.String(), `"status":"available"`) || !strings.Contains(list.Body.String(), `"status":"locked"`) {
		t.Fatalf("initial modules: %d %s", list.Code, list.Body.String())
	}

	moduleID := "83000000-0000-4000-8000-000000000111"
	lockedID := "83000000-0000-4000-8000-000000000112"
	negotiationHandler := NewNegotiationHandler(negotiation.NewService(db, llm.NewMock(), negotiation.Config{}, slog.Default()))
	router = NewRouter(NewHealthHandler(fakePinger{}, time.Second), []string{"*"}, nil, NewLearningHandler(learning.NewService(db, slog.Default())), negotiationHandler, pass, pass, NewUserHandler(nil), nil)
	assessmentPath := "/api/v1/companies/" + companyA + "/modules/" + moduleID + "/assessment-sessions"
	if early := request(http.MethodPost, assessmentPath, `{}`, &principal); early.Code != http.StatusConflict {
		t.Fatalf("assessment before steps: %d %s", early.Code, early.Body.String())
	}

	if got := request(http.MethodGet, listPath+"/"+lockedID, "", &principal).Code; got != 404 {
		t.Fatalf("locked module status=%d", got)
	}

	module := request(http.MethodGet, listPath+"/"+moduleID, "", &principal)
	if module.Code != 200 || strings.Contains(module.Body.String(), "answer_key") || strings.Contains(module.Body.String(), "private_config") || strings.Contains(module.Body.String(), "rubric") {
		t.Fatalf("public module response: %d %s", module.Code, module.Body.String())
	}

	step1 := "/api/v1/companies/" + companyA + "/steps/83000000-0000-4000-8000-000000000121/attempts"
	step2 := "/api/v1/companies/" + companyA + "/steps/83000000-0000-4000-8000-000000000122/attempts"
	if got := request(http.MethodPost, "/api/v1/companies/"+companyA+"/steps/not-a-uuid/attempts", `{"client_attempt_id":"bad-path","answer":"confirmed"}`, &principal).Code; got != 404 {
		t.Fatalf("invalid step path status=%d", got)
	}

	if got := request(http.MethodPost, step2, `{"client_attempt_id":"early","answer":"b"}`, &principal).Code; got != 409 {
		t.Fatalf("out-of-order step status=%d", got)
	}

	if got := request(http.MethodPost, step1, `{"client_attempt_id":"theory","answer":"confirmed"}`, &principal).Code; got != 200 {
		t.Fatalf("theory status=%d", got)
	}

	wrong := request(http.MethodPost, step2, `{"client_attempt_id":"wrong","answer":"a"}`, &principal)
	if wrong.Code != 200 || !strings.Contains(wrong.Body.String(), `"correct":false`) {
		t.Fatalf("wrong choice: %d %s", wrong.Code, wrong.Body.String())
	}

	repeat := request(http.MethodPost, step2, `{"client_attempt_id":"wrong","answer":"a"}`, &principal)
	if repeat.Code != 200 || repeat.Body.String() != wrong.Body.String() {
		t.Fatalf("idempotent replay: %d %s", repeat.Code, repeat.Body.String())
	}

	if got := request(http.MethodPost, step2, `{"client_attempt_id":"wrong","answer":"b"}`, &principal).Code; got != 409 {
		t.Fatalf("changed payload status=%d", got)
	}

	if got := request(http.MethodPost, step2, `{"client_attempt_id":"right","answer":"b"}`, &principal).Code; got != 200 {
		t.Fatalf("correct choice status=%d", got)
	}

	router = NewRouter(NewHealthHandler(fakePinger{}, time.Second), []string{"*"}, nil, NewLearningHandler(learning.NewService(db, slog.Default())), nil, pass, pass, NewUserHandler(nil), nil)
	module = request(http.MethodGet, listPath+"/"+moduleID, "", &principal)

	var restored learning.ModuleDetail

	if err := json.Unmarshal(module.Body.Bytes(), &restored); err != nil {
		t.Fatal(err)
	}

	if restored.CompletedSteps != 2 || !restored.Steps[0].Completed || !restored.Steps[1].Completed {
		t.Fatalf("restored state: %+v", restored)
	}

	var attempts int64

	if err := db.Model(&model.LearningAttempt{}).Where("company_id = ? AND user_id = ?", companyA, userA).Count(&attempts).Error; err != nil || attempts != 3 {
		t.Fatalf("attempt count=%d, err=%v", attempts, err)
	}

	for i, item := range []struct{ step, answer string }{
		{"83000000-0000-4000-8000-000000000123", "confirmed"},
		{"83000000-0000-4000-8000-000000000124", "b"},
		{"83000000-0000-4000-8000-000000000125", "confirmed"},
		{"83000000-0000-4000-8000-000000000126", "b"},
	} {
		path := "/api/v1/companies/" + companyA + "/steps/" + item.step + "/attempts"
		body := fmt.Sprintf(`{"client_attempt_id":"later-%d","answer":"%s"}`, i, item.answer)
		response := request(http.MethodPost, path, body, &principal)
		if response.Code != 200 {
			t.Fatalf("step %d: %d %s", i+3, response.Code, response.Body.String())
		}

		if i == 3 && !strings.Contains(response.Body.String(), `"module_status":"assessment_ready"`) {
			t.Fatalf("final step status: %s", response.Body.String())
		}
	}

	if got := request(http.MethodPost, step2, `{"client_attempt_id":"after-completion","answer":"b"}`, &principal).Code; got != 409 {
		t.Fatalf("completed step accepted again: %d", got)
	}

	router = NewRouter(NewHealthHandler(fakePinger{}, time.Second), []string{"*"}, nil, NewLearningHandler(learning.NewService(db, slog.Default())), negotiationHandler, pass, pass, NewUserHandler(nil), nil)
	start := request(http.MethodPost, assessmentPath, `{}`, &principal)
	if start.Code != http.StatusCreated {
		t.Fatalf("start assessment: %d %s", start.Code, start.Body.String())
	}

	var assessmentSession negotiation.SessionView

	if err := json.Unmarshal(start.Body.Bytes(), &assessmentSession); err != nil {
		t.Fatal(err)
	}

	messagePath := "/api/v1/companies/" + companyA + "/sessions/" + assessmentSession.SessionID + "/messages"
	messageBody := `{"client_message_id":"first","text":"Мы постараемся доставить весь заказ как можно скорее."}`
	firstTurn := request(http.MethodPost, messagePath, messageBody, &principal)
	if firstTurn.Code != http.StatusOK {
		t.Fatalf("first turn: %d %s", firstTurn.Code, firstTurn.Body.String())
	}

	if replay := request(http.MethodPost, messagePath, messageBody, &principal); replay.Code != http.StatusOK || replay.Body.String() != firstTurn.Body.String() {
		t.Fatalf("message replay: %d %s", replay.Code, replay.Body.String())
	}

	if changed := request(http.MethodPost, messagePath, `{"client_message_id":"first","text":"Иной ответ"}`, &principal); changed.Code != http.StatusConflict {
		t.Fatalf("message key reuse: %d", changed.Code)
	}

	finishPath := "/api/v1/companies/" + companyA + "/sessions/" + assessmentSession.SessionID + "/finish"
	failed := request(http.MethodPost, finishPath, `{}`, &principal)
	if failed.Code != http.StatusOK || !strings.Contains(failed.Body.String(), `"module_status":"needs_retry"`) {
		t.Fatalf("failed assessment: %d %s", failed.Code, failed.Body.String())
	}

	if next := request(http.MethodGet, listPath+"/"+lockedID, "", &principal); next.Code != http.StatusNotFound {
		t.Fatalf("next module unlocked by fail: %d", next.Code)
	}

	if repeated := request(http.MethodPost, finishPath, `{}`, &principal); repeated.Code != http.StatusOK || repeated.Body.String() != failed.Body.String() {
		t.Fatalf("repeat finish: %d %s", repeated.Code, repeated.Body.String())
	}

	router = NewRouter(NewHealthHandler(fakePinger{}, time.Second), []string{"*"}, nil, NewLearningHandler(learning.NewService(db, slog.Default())),
		NewNegotiationHandler(negotiation.NewService(db, invalidAssessmentProvider{mock: llm.NewMock()}, negotiation.Config{}, slog.Default())),
		pass, pass, NewUserHandler(nil), nil)

	retry := request(http.MethodPost, "/api/v1/companies/"+companyA+"/sessions/"+assessmentSession.SessionID+"/retry", `{}`, &principal)
	if retry.Code != http.StatusCreated {
		t.Fatalf("retry assessment: %d %s", retry.Code, retry.Body.String())
	}

	if err := json.Unmarshal(retry.Body.Bytes(), &assessmentSession); err != nil {
		t.Fatal(err)
	}

	messagePath = "/api/v1/companies/" + companyA + "/sessions/" + assessmentSession.SessionID + "/messages"

	for i, line := range []string{
		"Почему срок поставки именно через пять дней?",
		"Верно ли, что к открытию нужны только 20 комплектов?",
		"Предлагаю 20 комплектов за 5 дней и остальные 80 за 30 дней.",
	} {
		body, _ := json.Marshal(map[string]string{"client_message_id": fmt.Sprintf("pass-%d", i), "text": line})
		turn := request(http.MethodPost, messagePath, string(body), &principal)
		if turn.Code != http.StatusOK {
			t.Fatalf("passing turn %d: %d %s", i, turn.Code, turn.Body.String())
		}
	}

	finishPath = "/api/v1/companies/" + companyA + "/sessions/" + assessmentSession.SessionID + "/finish"
	degraded := request(http.MethodPost, finishPath, `{}`, &principal)
	if degraded.Code != http.StatusOK || !strings.Contains(degraded.Body.String(), `"evaluation_status":"needs_retry"`) {
		t.Fatalf("invalid model result: %d %s", degraded.Code, degraded.Body.String())
	}

	var assessmentCount int64

	if err := db.Model(&model.Assessment{}).Where("session_id = ?", assessmentSession.SessionID).Count(&assessmentCount).Error; err != nil || assessmentCount != 0 {
		t.Fatalf("invalid assessment persisted: count=%d err=%v", assessmentCount, err)
	}

	router = NewRouter(NewHealthHandler(fakePinger{}, time.Second), []string{"*"}, nil, NewLearningHandler(learning.NewService(db, slog.Default())), negotiationHandler, pass, pass, NewUserHandler(nil), nil)

	passed := request(http.MethodPost, finishPath, `{}`, &principal)
	if passed.Code != http.StatusOK || !strings.Contains(passed.Body.String(), `"module_status":"passed"`) {
		t.Fatalf("passed assessment: %d %s", passed.Code, passed.Body.String())
	}

	if next := request(http.MethodGet, listPath+"/"+lockedID, "", &principal); next.Code != http.StatusOK {
		t.Fatalf("next module after pass: %d %s", next.Code, next.Body.String())
	}

	for i, answer := range []string{"confirmed", "b", "confirmed", "b", "confirmed", "b"} {
		stepID := fmt.Sprintf("83000000-0000-4000-8000-%012d", 131+i)
		body, _ := json.Marshal(map[string]string{"client_attempt_id": fmt.Sprintf("module-two-%d", i), "answer": answer})
		response := request(http.MethodPost, "/api/v1/companies/"+companyA+"/steps/"+stepID+"/attempts", string(body), &principal)
		if response.Code != http.StatusOK {
			t.Fatalf("module 2 step %d: %d %s", i+1, response.Code, response.Body.String())
		}
	}

	secondAssessment := request(http.MethodPost, "/api/v1/companies/"+companyA+"/modules/"+lockedID+"/assessment-sessions", `{}`, &principal)
	if secondAssessment.Code != http.StatusCreated {
		t.Fatalf("module 2 assessment: %d %s", secondAssessment.Code, secondAssessment.Body.String())
	}

	if err := json.Unmarshal(secondAssessment.Body.Bytes(), &assessmentSession); err != nil {
		t.Fatal(err)
	}

	for i, line := range []string{
		"Если квартальный объём 200 единиц, обсудим предоплату 40 процентов.",
		"Наша альтернатива 1,10 млн и аванс 30 процентов лучше вашего пакета.",
		"Если не согласуем аванс 30 процентов, выберем альтернативу.",
	} {
		body, _ := json.Marshal(map[string]string{"client_message_id": fmt.Sprintf("module-two-turn-%d", i), "text": line})
		response := request(http.MethodPost, "/api/v1/companies/"+companyA+"/sessions/"+assessmentSession.SessionID+"/messages", string(body), &principal)
		if response.Code != http.StatusOK {
			t.Fatalf("module 2 turn %d: %d %s", i, response.Code, response.Body.String())
		}
	}

	secondResult := request(http.MethodPost, "/api/v1/companies/"+companyA+"/sessions/"+assessmentSession.SessionID+"/finish", `{}`, &principal)
	if secondResult.Code != http.StatusOK || !strings.Contains(secondResult.Body.String(), `"module_status":"passed"`) {
		t.Fatalf("module 2 result: %d %s", secondResult.Code, secondResult.Body.String())
	}

	practice := request(http.MethodPost, "/api/v1/companies/"+companyA+"/sessions", `{"case_key":"practice-procurement"}`, &principal)
	if practice.Code != http.StatusCreated {
		t.Fatalf("practice start: %d %s", practice.Code, practice.Body.String())
	}

	var practiceSession negotiation.SessionView

	if err := json.Unmarshal(practice.Body.Bytes(), &practiceSession); err != nil {
		t.Fatal(err)
	}

	practiceResult := request(http.MethodPost, "/api/v1/companies/"+companyA+"/sessions/"+practiceSession.SessionID+"/finish", `{}`, &principal)
	if practiceResult.Code != http.StatusOK || strings.Contains(practiceResult.Body.String(), `"module_status":`) {
		t.Fatalf("practice changed module progress: %d %s", practiceResult.Code, practiceResult.Body.String())
	}

	barrier := &barrierProvider{mock: llm.NewMock(), ready: make(chan struct{})}
	concurrentService := negotiation.NewService(db, barrier, negotiation.Config{}, slog.Default())
	concurrentSession, err := concurrentService.StartSession(context.Background(), companyA, userA, "practice-sales")
	if err != nil {
		t.Fatal(err)
	}

	type sendResult struct{ err error }

	results := make(chan sendResult, 2)

	for _, key := range []string{"concurrent-a", "concurrent-b"} {
		go func(id string) {
			_, sendErr := concurrentService.SendMessage(context.Background(), companyA, userA, concurrentSession.SessionID, id, "Давайте обсудим условия")

			results <- sendResult{err: sendErr}
		}(key)
	}

	successes, conflicts := 0, 0

	for range 2 {
		result := <-results

		switch {
		case result.err == nil:
			successes++

		case errors.Is(result.err, negotiation.ErrConflict):
			conflicts++

		default:
			t.Fatalf("concurrent send: %v", result.err)
		}
	}

	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent sends: successes=%d conflicts=%d", successes, conflicts)
	}

	var messageCount int64

	if err := db.Model(&model.Message{}).Where("session_id = ?", concurrentSession.SessionID).Count(&messageCount).Error; err != nil || messageCount != 3 {
		t.Fatalf("concurrent messages: count=%d err=%v", messageCount, err)
	}

	if repeated := request(http.MethodPost, finishPath, `{}`, &principal); repeated.Code != http.StatusOK || repeated.Body.String() != passed.Body.String() {
		t.Fatalf("repeat passed finish: %d %s", repeated.Code, repeated.Body.String())
	}

	if denied := request(http.MethodPost, "/api/v1/companies/"+companyA+"/sessions/"+assessmentSession.SessionID+"/retry", `{}`, &principal); denied.Code != http.StatusConflict {
		t.Fatalf("retry after pass: %d", denied.Code)
	}

	otherUser := identity.Principal{UserID: userB, CompanyID: companyA, Role: "employee", Active: true}
	otherCompany := identity.Principal{UserID: userA, CompanyID: companyB, Role: "employee", Active: true}

	for _, tc := range []struct {
		path string
		p    identity.Principal
	}{
		{listPath, otherUser},
		{"/api/v1/companies/" + companyB + "/modules", otherCompany},
	} {
		response := request(http.MethodGet, tc.path, "", &tc.p)
		if response.Code != 200 || !strings.Contains(response.Body.String(), `"completed_steps":0`) {
			t.Fatalf("tenant isolation: %d %s", response.Code, response.Body.String())
		}
	}
}

type invalidAssessmentProvider struct{ mock *llm.Mock }

func (p invalidAssessmentProvider) Complete(ctx context.Context, req llm.Request) (string, error) {
	if req.Kind == llm.KindAssessment {
		return "{invalid", nil
	}

	return p.mock.Complete(ctx, req)
}

type barrierProvider struct {
	mock  *llm.Mock
	ready chan struct{}
	count atomic.Int32
}

func (p *barrierProvider) Complete(ctx context.Context, req llm.Request) (string, error) {
	if req.Kind == llm.KindDialogue && req.Turn > 0 {
		if p.count.Add(1) == 2 {
			close(p.ready)
		}

		select {
		case <-p.ready:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	return p.mock.Complete(ctx, req)
}
