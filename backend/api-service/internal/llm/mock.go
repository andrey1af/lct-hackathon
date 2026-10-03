package llm

import (
	"context"
	"embed"
	"encoding/json"
	"path"
	"strings"
)

//go:embed scripts/*.json
var scriptFiles embed.FS

const (
	metComment   = "Действие подтверждено репликой сотрудника."
	unmetComment = "В репликах сотрудника нет подтверждения этого действия."
	emptyComment = "Сотрудник не отправил ни одной реплики."
)

const evidenceLimit = 140

// Each alternative requires every fragment in one player line. Unknown replies prove no criterion.
var criterionSignals = map[string][][]string{
	"interest_question":         {{"почему", "срок"}, {"что", "через пять дней"}, {"сколько", "к открытию"}},
	"checks_understanding":      {{"верно", "20"}, {"правильно", "20"}},
	"uses_interest":             {{"20", "5", "80", "30"}},
	"conditional_trade":         {{"если", "200", "40"}, {"при", "200", "40"}},
	"batna_comparison":          {{"альтернатива", "1,10", "30"}, {"альтернатива", "1100000", "30"}},
	"rational_decision":         {{"выберем альтернативу", "30"}, {"откажемся", "альтернатив"}},
	"partial_delivery_question": {{"2000", "14"}, {"2 000", "14"}},
	"measurable_trade":          {{"если", "10 000", "118"}, {"если", "10000", "118"}},
	"alternative_check":         {{"альтернатива", "118", "25"}, {"альтернативой", "118", "25"}},
	"launch_interest":           {{"сколько", "первую неделю"}, {"кому", "семь дней"}},
	"conditional_discount":      {{"два года", "840"}, {"двухлет", "840"}},
	"sales_batna":               {{"ниже", "820", "не подходит"}, {"меньше", "820", "откаж"}},
	"objection_question":        {{"входит", "конкурент"}, {"что включено", "конкурент"}},
	"argument_fit":              {{"поддержка", "24/7"}, {"поддержку", "24/7"}},
	"price_discipline":          {{"1 056 000", "не дадим"}, {"1 056 000", "без сокращения"}},
	"cross_issue_trade":         {{"три года", "10 800"}, {"трехлетн", "10 800"}},
	"package_comparison":        {{"альтернатив", "11 200"}},
	"next_step":                 {{"протокол", "до пятницы"}, {"протокол", "до среды"}},
	"cost_breakdown_question":   {{"какая часть", "топлив"}, {"из чего", "повышение"}},
	"payment_trade":             {{"7 дней", "74 000"}, {"семь дней", "74 000"}},
	"carrier_batna":             {{"альтернатив", "76 000"}},
	"brand_interest_question":   {{"почему", "бренд"}, {"зачем", "первое место"}},
	"value_trade":               {{"бренд", "50"}, {"первое место", "запис"}},
	"launch_next_step":          {{"зафиксируем", "ответственн"}, {"дата", "ответственн"}},
}

var generic = script{
	Opening: "Добрый день. У нас предложение на столе, и менять его без веских причин я не готов. С чем вы пришли?",
	Replies: []string{
		"Это предложение меня не убеждает. Что вы готовы предложить взамен?",
		"Допустим. Но тогда мне нужно понимать, что я получаю с вашей стороны.",
		"Давайте по существу: какое условие вы меняете и на каких основаниях?",
		"Я готов обсуждать пакет целиком, а не одну цифру. Сформулируйте предложение.",
	},
	Hint:          "Назовите вслух то, что уже услышали от собеседника, и задайте один вопрос о причине его требования.",
	WhatWorked:    "Диалог состоялся, вы удерживали тему переговоров.",
	WhatToTryNext: "Попробуйте связать каждую свою уступку со встречным измеримым условием.",
}

type script struct {
	CaseKey       string   `json:"case_key"`
	Opening       string   `json:"opening"`
	Replies       []string `json:"replies"`
	Hint          string   `json:"hint"`
	Unmet         []string `json:"unmet"`
	WhatWorked    string   `json:"what_worked"`
	WhatToTryNext string   `json:"what_to_try_next"`
}

type Mock struct {
	scripts map[string]script
}

func NewMock() *Mock {
	mock := &Mock{scripts: make(map[string]script)}

	entries, err := scriptFiles.ReadDir("scripts")
	if err != nil {
		return mock
	}

	for _, entry := range entries {
		raw, err := scriptFiles.ReadFile(path.Join("scripts", entry.Name()))
		if err != nil {
			continue
		}

		var parsed script

		if err := json.Unmarshal(raw, &parsed); err != nil || parsed.CaseKey == "" || len(parsed.Replies) == 0 {
			continue
		}

		mock.scripts[parsed.CaseKey] = parsed
	}

	return mock
}

func (m *Mock) Complete(_ context.Context, req Request) (string, error) {
	selected := m.script(req.CaseKey)

	switch req.Kind {
	case KindAssessment:
		return verdictOf(selected, req), nil

	case KindHint:
		if selected.Hint == "" {
			return generic.Hint, nil
		}

		return selected.Hint, nil

	default:
		return replyOf(selected, req.Turn), nil
	}
}

func (m *Mock) script(caseKey string) script {
	selected, ok := m.scripts[caseKey]
	if !ok {
		return generic
	}

	if selected.Opening == "" {
		selected.Opening = generic.Opening
	}

	return selected
}

func replyOf(s script, turn int) string {
	if turn <= 0 {
		return s.Opening
	}

	return s.Replies[(turn-1)%len(s.Replies)]
}

type mockCriterion struct {
	ID       string `json:"id"`
	Met      bool   `json:"met"`
	Evidence string `json:"evidence"`
	Comment  string `json:"comment"`
}

type mockVerdict struct {
	Criteria      []mockCriterion `json:"criteria"`
	WhatWorked    string          `json:"what_worked"`
	WhatToTryNext string          `json:"what_to_try_next"`
}

func verdictOf(s script, req Request) string {
	result := mockVerdict{
		Criteria:      make([]mockCriterion, 0, len(req.CriteriaIDs)),
		WhatWorked:    orDefault(s.WhatWorked, generic.WhatWorked),
		WhatToTryNext: orDefault(s.WhatToTryNext, generic.WhatToTryNext),
	}

	for _, id := range req.CriteriaIDs {
		item := mockCriterion{ID: id, Comment: unmetComment}

		if len(req.PlayerLines) == 0 {
			item.Comment = emptyComment
		} else if line := matchingEvidence(id, req.PlayerLines); line != "" {
			item.Met, item.Evidence, item.Comment = true, quote(line), metComment
		}

		result.Criteria = append(result.Criteria, item)
	}

	if len(req.PlayerLines) == 0 {
		result.WhatWorked = ""
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		return `{"criteria":[]}`
	}

	return string(encoded)
}

func matchingEvidence(id string, lines []string) string {
	for _, line := range lines {
		lower := strings.ToLower(strings.ReplaceAll(line, "ё", "е"))

		for _, alternative := range criterionSignals[id] {
			matched := true

			for _, fragment := range alternative {
				if !strings.Contains(lower, fragment) {
					matched = false
					break
				}
			}

			if matched {
				return line
			}
		}
	}

	return ""
}

func quote(line string) string {
	runes := []rune(strings.TrimSpace(line))
	if len(runes) <= evidenceLimit {
		return string(runes)
	}

	cut := string(runes[:evidenceLimit])
	if space := strings.LastIndexByte(cut, ' '); space > 0 {
		cut = cut[:space]
	}

	return cut
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}

	return value
}
