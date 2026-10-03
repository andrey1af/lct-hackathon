package negotiation

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/llm"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/model"
)

type assessmentReplies struct {
	replies []string
	calls   int
}

func (p *assessmentReplies) Complete(_ context.Context, req llm.Request) (string, error) {
	p.calls++
	if p.calls == 2 && !strings.Contains(req.Messages[len(req.Messages)-1].Content, "Предыдущий ответ") {
		return "", context.Canceled
	}

	return p.replies[p.calls-1], nil
}

func TestJudgeRetriesMalformedVerdict(t *testing.T) {
	const evidence = "Предлагаю обсудить сроки поставки"

	r := rubric{Criteria: []rubricCriterion{{ID: "terms", Behavior: "Обсудить сроки", Required: true}}}
	messages := []model.Message{{Role: roleUser, Text: evidence, TurnNumber: 1}}
	valid := `{"criteria":[{"id":"terms","met":true,"evidence":"` + evidence + `","comment":"Обсудил сроки."}],"what_worked":"Обсуждение сроков.","what_to_try_next":"Уточнить дату."}`

	for _, tc := range []struct {
		name       string
		replies    []string
		wantStatus string
		wantMet    bool
	}{
		{"second response valid", []string{`{"criteria":[{"id":"terms"],"what_worked":""}`, valid}, evaluationCompleted, true},
		{"both responses invalid", []string{`{"criteria":[}`, `{"criteria":[}`}, evaluationDegraded, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &assessmentReplies{replies: tc.replies}
			service := NewService(nil, provider, Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
			criteria, _, status := service.judge(context.Background(), "case", model.CaseVersion{}, r, messages)
			if provider.calls != 2 || status != tc.wantStatus || len(criteria) != 1 || criteria[0].Met != tc.wantMet {
				t.Fatalf("calls=%d status=%s criteria=%+v", provider.calls, status, criteria)
			}
		})
	}
}

func TestJudgeRepairsMalformedMetFields(t *testing.T) {
	const question = "Что произойдёт через пять дней и сколько комплектов нужно к этой дате?"

	const check = "Подскажите, пожалуйста, такой график отгрузки полностью закрывает ваши потребности по проекту?"

	r := rubric{Criteria: []rubricCriterion{
		{ID: "interest_question", Required: true},
		{ID: "checks_understanding", Required: true},
		{ID: "uses_interest", Required: true},
	}}
	answer := `{"criteria":[` +
		`{"id":"interest_question","met":true,"evidence":"` + question + `","comment":"Задал вопрос."},` +
		`{"id":"checks_understanding","met:true","evidence":"` + check + `","comment":"Проверил понимание."},` +
		`{"id":"uses_interest","met:false,"evidence":"","comment":"Не использовал интерес."}` +
		`],"what_worked":"Выяснил срочность.","what_to_try_next":"Предложить частичную поставку."}`
	provider := &assessmentReplies{replies: []string{answer}}
	service := NewService(nil, provider, Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	messages := []model.Message{{Role: roleUser, Text: question, TurnNumber: 1}, {Role: roleUser, Text: check, TurnNumber: 2}}

	criteria, feedback, status := service.judge(context.Background(), "assessment-interests", model.CaseVersion{}, r, messages)
	if provider.calls != 1 || status != evaluationCompleted || len(criteria) != 3 || !criteria[0].Met || !criteria[1].Met || criteria[2].Met {
		t.Fatalf("calls=%d status=%s criteria=%+v", provider.calls, status, criteria)
	}

	if feedback.WhatWorked != "Выяснил срочность." {
		t.Fatalf("feedback=%+v", feedback)
	}
}
