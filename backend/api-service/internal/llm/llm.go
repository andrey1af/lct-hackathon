package llm

import (
	"context"
	"log/slog"
	"strings"
)

const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

const (
	KindDialogue   = "dialogue"
	KindAssessment = "assessment"
	KindHint       = "hint"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	Kind        string
	CaseKey     string
	Turn        int
	CriteriaIDs []string
	PlayerLines []string
	Messages    []Message
}

type Provider interface {
	Complete(ctx context.Context, req Request) (string, error)
}

type chain struct {
	primary   Provider
	secondary Provider
	log       *slog.Logger
}

func (c chain) Complete(ctx context.Context, req Request) (string, error) {
	const op = "llmChain.Complete"

	log := c.log.With(slog.String("op", op), slog.String("kind", req.Kind), slog.String("case_key", req.CaseKey))

	text, err := c.primary.Complete(ctx, req)
	if err == nil {
		return text, nil
	}

	log.ErrorContext(ctx, "llm provider failed, serving a scripted reply", slog.Any("error", err))

	return c.secondary.Complete(ctx, req)
}

func snippet(body []byte) string {
	text := strings.TrimSpace(string(body))
	if len(text) > 300 {
		text = text[:300]
	}

	return text
}
