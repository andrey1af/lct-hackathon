package negotiation

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/llm"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/model"
)

const (
	evaluationCompleted = "evaluated"
	evaluationDegraded  = "needs_retry"
)

const degradedComment = "Автоматическая оценка недоступна, критерий не подтверждён."

const degradedAdvice = "Оценка не была получена по техническим причинам. Попробуйте пройти кейс ещё раз."

const unverifiedComment = "Модель не привела цитату из реплик сотрудника, поэтому критерий не засчитан."

const minEvidenceRunes = 12

var malformedMetField = regexp.MustCompile(`([,{]\s*)"met\s*:\s*(true|false)"?`)

type rubric struct {
	CaseKey  string            `json:"case_key"`
	Version  int               `json:"version"`
	Criteria []rubricCriterion `json:"criteria"`
}

type rubricCriterion struct {
	ID             string `json:"id"`
	Required       bool   `json:"required"`
	Behavior       string `json:"behavior"`
	SuccessExample string `json:"success_example"`
	FailureExample string `json:"failure_example"`
	Evidence       string `json:"evidence"`
}

type CriterionView struct {
	ID       string `json:"id"`
	Behavior string `json:"behavior"`
	Required bool   `json:"required"`
	Met      bool   `json:"met"`
	Evidence string `json:"evidence"`
	Comment  string `json:"comment"`
}

type Feedback struct {
	WhatWorked    string `json:"what_worked"`
	WhatToTryNext string `json:"what_to_try_next"`
}

type ResultView struct {
	SessionID        string          `json:"session_id"`
	Status           string          `json:"status"`
	Mode             string          `json:"mode"`
	CaseKey          string          `json:"case_key"`
	Title            string          `json:"title"`
	Passed           bool            `json:"passed"`
	EvaluationStatus string          `json:"evaluation_status"`
	ModuleStatus     string          `json:"module_status,omitempty"`
	RubricVersion    int             `json:"rubric_version"`
	Criteria         []CriterionView `json:"criteria"`
	Feedback         Feedback        `json:"feedback"`
	Attempt          int             `json:"attempt"`
	TurnsUsed        int             `json:"turns_used"`
	MaxTurns         int             `json:"max_turns"`
	CanRetry         bool            `json:"can_retry"`
}

type verdict struct {
	Criteria []struct {
		ID       string `json:"id"`
		Met      bool   `json:"met"`
		Evidence string `json:"evidence"`
		Comment  string `json:"comment"`
	} `json:"criteria"`
	WhatWorked    string `json:"what_worked"`
	WhatToTryNext string `json:"what_to_try_next"`
}

func parseRubric(raw json.RawMessage) (rubric, error) {
	var parsed rubric

	if err := json.Unmarshal(raw, &parsed); err != nil {
		return rubric{}, fmt.Errorf("decode rubric: %w", err)
	}

	if len(parsed.Criteria) == 0 {
		return rubric{}, fmt.Errorf("rubric has no criteria")
	}

	return parsed, nil
}

func assessmentRequest(caseKey string, r rubric, caseVersion model.CaseVersion, messages []model.Message) llm.Request {
	var b strings.Builder

	b.WriteString("Ты — методист, который разбирает учебные переговоры. Оцени только действия Сотрудника.\n")
	fmt.Fprintf(&b, "Кейс: %s.\n", caseVersion.Title)
	fmt.Fprintf(&b, "Задание сотрудника: %s\n", caseVersion.Briefing)

	b.WriteString("\nКритерии оценки:\n")

	for _, criterion := range r.Criteria {
		fmt.Fprintf(&b, "- id %q: %s. Пример выполнения: %s. Пример невыполнения: %s.\n",
			criterion.ID, criterion.Behavior, criterion.SuccessExample, criterion.FailureExample)
	}

	b.WriteString(`
Правила оценки:
- Критерий выполнен только если в репликах Сотрудника есть прямое подтверждение. Сомнение — критерий не выполнен.
- В поле evidence приводи короткую цитату из реплики Сотрудника, на которой основан вывод. Если подтверждения нет, оставь evidence пустым.
- Оценивай действия, а не личность: никаких оценок характера, способностей и мотивов.
- Не выставляй никаких баллов, процентов и общих оценок.
- Отвечай на русском языке.
- Поле met должно быть логическим значением true или false, без кавычек.

Верни строго JSON по схеме, без markdown и пояснений:
{"criteria":[{"id":"<id критерия>","met":false,"evidence":"<цитата или пустая строка>","comment":"<1-2 предложения о наблюдаемом действии>"}],"what_worked":"<что сработало, 1-2 предложения>","what_to_try_next":"<что попробовать в следующий раз, 1-2 предложения>"}`)

	ids := make([]string, 0, len(r.Criteria))

	for _, criterion := range r.Criteria {
		ids = append(ids, criterion.ID)
	}

	return llm.Request{
		Kind:        llm.KindAssessment,
		CaseKey:     caseKey,
		Turn:        playerTurns(messages),
		CriteriaIDs: ids,
		PlayerLines: playerLines(messages),
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: b.String()},
			{Role: llm.RoleUser, Content: "Стенограмма переговоров:\n\n" + transcript(messages)},
		},
	}
}

func playerLines(messages []model.Message) []string {
	lines := make([]string, 0, len(messages))

	for _, message := range messages {
		if message.Role == roleUser {
			lines = append(lines, message.Text)
		}
	}

	return lines
}

func normalize(text string) string {
	var b strings.Builder

	previousSpace := true

	for _, r := range strings.ToLower(text) {
		switch {
		case r == 'ё':
			b.WriteRune('е')

			previousSpace = false

		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)

			previousSpace = false

		case !previousSpace:
			b.WriteByte(' ')

			previousSpace = true
		}
	}

	return strings.TrimSpace(b.String())
}

func evidenceConfirmed(evidence string, lines []string) bool {
	needle := normalize(evidence)
	if len([]rune(needle)) < minEvidenceRunes {
		return false
	}

	for _, line := range lines {
		if strings.Contains(normalize(line), needle) {
			return true
		}
	}

	return false
}

func transcript(messages []model.Message) string {
	var b strings.Builder

	for _, message := range messages {
		speaker := "Сотрудник"
		if message.Role == roleOpponent {
			speaker = "Оппонент"
		}

		fmt.Fprintf(&b, "Ход %d — %s: %s\n", message.TurnNumber, speaker, message.Text)
	}

	return b.String()
}

func parseVerdict(answer string, r rubric, lines []string) ([]CriterionView, Feedback, error) {
	parsed, err := decodeVerdict(answer)
	if err != nil {
		return nil, Feedback{}, err
	}

	byID := make(map[string]int, len(parsed.Criteria))

	for i, criterion := range parsed.Criteria {
		byID[criterion.ID] = i
	}

	views := make([]CriterionView, 0, len(r.Criteria))

	for _, criterion := range r.Criteria {
		view := CriterionView{ID: criterion.ID, Behavior: criterion.Behavior, Required: criterion.Required}

		if i, ok := byID[criterion.ID]; ok {
			view.Met = parsed.Criteria[i].Met
			view.Evidence = strings.TrimSpace(parsed.Criteria[i].Evidence)
			view.Comment = strings.TrimSpace(parsed.Criteria[i].Comment)
		}

		if !evidenceConfirmed(view.Evidence, lines) {
			view.Evidence = ""

			if view.Met {
				view.Met = false
				view.Comment = unverifiedComment
			}
		}

		views = append(views, view)
	}

	feedback := Feedback{
		WhatWorked:    strings.TrimSpace(parsed.WhatWorked),
		WhatToTryNext: strings.TrimSpace(parsed.WhatToTryNext),
	}

	return views, feedback, nil
}

func decodeVerdict(answer string) (verdict, error) {
	candidates := []string{answer}
	repaired := malformedMetField.ReplaceAllString(answer, `${1}"met":${2}`)
	if repaired != answer {
		candidates = append(candidates, repaired)
	}

	var decodeErr error

	for _, candidate := range candidates {
		object := jsonObject(candidate)
		if object == "" {
			continue
		}

		var parsed verdict

		if err := json.Unmarshal([]byte(object), &parsed); err == nil {
			return parsed, nil
		} else {
			decodeErr = err
		}
	}

	if decodeErr != nil {
		return verdict{}, fmt.Errorf("decode verdict: %w", decodeErr)
	}

	return verdict{}, fmt.Errorf("decode verdict: no JSON object in answer")
}

func degradedVerdict(r rubric) ([]CriterionView, Feedback) {
	views := make([]CriterionView, 0, len(r.Criteria))

	for _, criterion := range r.Criteria {
		views = append(views, CriterionView{
			ID:       criterion.ID,
			Behavior: criterion.Behavior,
			Required: criterion.Required,
			Comment:  degradedComment,
		})
	}

	return views, Feedback{WhatToTryNext: degradedAdvice}
}

func passed(criteria []CriterionView) bool {
	anyRequired, anyMet := false, false

	for _, criterion := range criteria {
		if criterion.Met {
			anyMet = true
		}

		if criterion.Required {
			anyRequired = true

			if !criterion.Met {
				return false
			}
		}
	}

	if anyRequired {
		return true
	}

	return anyMet
}

func jsonObject(answer string) string {
	start := strings.IndexByte(answer, '{')
	if start < 0 {
		return ""
	}

	depth, inString, escaped := 0, false, false

	for i := start; i < len(answer); i++ {
		char := answer[i]

		switch {
		case escaped:
			escaped = false

		case char == '\\' && inString:
			escaped = true

		case char == '"':
			inString = !inString

		case inString:

		case char == '{':
			depth++

		case char == '}':
			depth--

			if depth == 0 {
				return answer[start : i+1]
			}
		}
	}

	return ""
}

func truncate(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}

	return string(runes[:limit]) + "…"
}
