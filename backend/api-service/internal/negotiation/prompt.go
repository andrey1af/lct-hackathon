package negotiation

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/llm"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/model"
)

const openingInstruction = "Начни переговоры: поздоровайся одной фразой и обозначь свою исходную позицию по условиям сделки."

const fallbackReply = "Мне нужно время, чтобы обдумать это. Давайте вернёмся к условиям — что именно вы предлагаете изменить?"

type issue struct {
	Value json.Number `json:"value"`
	Unit  string      `json:"unit"`
}

type privateConfig struct {
	OpponentRole   string           `json:"opponent_role"`
	HiddenInterest string           `json:"hidden_interest"`
	Issues         map[string]issue `json:"issues"`
}

func systemPrompt(caseVersion model.CaseVersion) string {
	var cfg privateConfig

	if err := json.Unmarshal(caseVersion.PrivateConfig, &cfg); err != nil {
		cfg = privateConfig{}
	}

	role := cfg.OpponentRole
	if role == "" {
		role = "представитель второй стороны в переговорах"
	}

	var b strings.Builder

	fmt.Fprintf(&b, "Ты — %s. Ты ведёшь переговоры с сотрудником другой компании в учебном тренажёре.\n", role)
	fmt.Fprintf(&b, "Тема переговоров: %s.\n", caseVersion.Title)

	if own := openingTerms(cfg.Issues); len(own) > 0 {
		b.WriteString("\nУсловия, с которых ты начинаешь и которые отстаиваешь:\n")

		for _, name := range slices.Sorted(maps.Keys(own)) {
			fmt.Fprintf(&b, "- %s: %s %s\n", name, own[name].Value.String(), own[name].Unit)
		}
	}

	if cfg.HiddenInterest != "" {
		b.WriteString("\nТвой скрытый интерес: " + cfg.HiddenInterest + "\n")
		b.WriteString("Сам его не раскрывай. Упомянуть его можно только если собеседник задал вопрос именно по этой теме.\n")
	}

	b.WriteString(`
Правила твоих ответов:
- Отвечай на русском языке, одной репликой от первого лица, 1-3 предложения.
- Не выходи из роли, не пиши пояснений, ремарок в скобках, markdown и списков.
- Не придумывай фактов и цифр, которых нет в условиях выше.
- Держи свою позицию: уступай только в обмен на встречное условие.
- Не оценивай собеседника, не давай ему советов и не подсказывай правильные ходы.
- Не упоминай, что ты языковая модель или что это тренажёр.`)

	return b.String()
}

func openingTerms(issues map[string]issue) map[string]issue {
	own := make(map[string]issue, len(issues))

	for name, value := range issues {
		if name == "quantity" || strings.HasPrefix(name, "opening_") {
			own[name] = value
		}
	}

	return own
}

const fallbackHint = "Перечитайте задание и назовите вслух то, что уже услышали от собеседника, а затем задайте один вопрос о причине его требования."

func openingRequest(caseKey string, caseVersion model.CaseVersion) llm.Request {
	return llm.Request{
		Kind:    llm.KindDialogue,
		CaseKey: caseKey,
		Turn:    0,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: systemPrompt(caseVersion)},
			{Role: llm.RoleUser, Content: openingInstruction},
		},
	}
}

func dialogueRequest(caseKey string, caseVersion model.CaseVersion, history []model.Message, text string, turn int) llm.Request {
	messages := []llm.Message{{Role: llm.RoleSystem, Content: systemPrompt(caseVersion)}}
	messages = append(messages, promptHistory(history)...)
	messages = append(messages, llm.Message{Role: llm.RoleUser, Content: text})

	return llm.Request{Kind: llm.KindDialogue, CaseKey: caseKey, Turn: turn, Messages: messages}
}

func hintRequest(caseKey string, caseVersion model.CaseVersion, r rubric, messages []model.Message) llm.Request {
	var b strings.Builder

	b.WriteString("Ты — тренер по переговорам. Сотрудник ведёт учебный диалог и просит подсказку.\n")
	fmt.Fprintf(&b, "Кейс: %s.\n", caseVersion.Title)
	fmt.Fprintf(&b, "Задание сотрудника: %s\n", caseVersion.Briefing)

	b.WriteString("\nДействия, которые разбираются после диалога:\n")

	for _, criterion := range r.Criteria {
		fmt.Fprintf(&b, "- %s\n", criterion.Behavior)
	}

	b.WriteString(`
Правила подсказки:
- Назови одно действие из списка, которого в репликах сотрудника пока не видно.
- Не давай готовую фразу для копирования и не пиши за сотрудника его реплику.
- Не раскрывай позицию, мотивы и скрытые условия оппонента.
- Оценивай действия, а не личность. Никаких баллов и оценок.
- Ответь на русском языке одним абзацем из 1-2 предложений, без markdown и списков.`)

	return llm.Request{
		Kind:        llm.KindHint,
		CaseKey:     caseKey,
		Turn:        playerTurns(messages),
		PlayerLines: playerLines(messages),
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: b.String()},
			{Role: llm.RoleUser, Content: "Ход диалога:\n\n" + transcript(messages)},
		},
	}
}
