package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"uuid"

	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/config"
)

var (
	errUnauthorized  = errors.New("gigachat rejected the access token")
	errNoChoices     = errors.New("gigachat returned no choices")
	errEmptyReply    = errors.New("gigachat returned an empty reply")
	errNoAccessToken = errors.New("gigachat token response contains no access_token")
)

func New(cfg config.GigaChatConfig, logger *slog.Logger) Provider {
	const op = "llm.New"

	if logger == nil {
		logger = slog.Default()
	}

	log := logger.With(slog.String("op", op))
	mock := NewMock()

	if cfg.BasicCredentials() == "" {
		log.Warn("GigaChat credentials are not configured, replies are scripted")

		return mock
	}

	log.Info("GigaChat provider enabled",
		slog.String("model", cfg.Model),
		slog.String("base_url", cfg.BaseURL),
		slog.String("scope", cfg.Scope),
	)

	live := &gigachat{
		cfg:  cfg,
		log:  logger,
		http: &http.Client{Timeout: cfg.RequestTimeout},
	}

	return chain{primary: live, secondary: mock, log: logger}
}

type gigachat struct {
	cfg  config.GigaChatConfig
	log  *slog.Logger
	http *http.Client

	mu     sync.Mutex
	token  string
	expiry time.Time
}

func (g *gigachat) Complete(ctx context.Context, req Request) (string, error) {
	const op = "gigachat.Complete"

	log := g.log.With(slog.String("op", op), slog.String("kind", req.Kind), slog.String("case_key", req.CaseKey))

	started := time.Now()

	text, err := g.complete(ctx, req)
	if err == nil {
		logReply(ctx, log, 1, started, text)

		return text, nil
	}

	log.WarnContext(ctx, "GigaChat call failed, retrying once", slog.Any("error", err))

	started = time.Now()

	text, retryErr := g.complete(ctx, req)
	if retryErr != nil {
		log.ErrorContext(ctx, "GigaChat retry failed", slog.Any("error", retryErr))

		return "", fmt.Errorf("%s: retry failed: %w (first attempt: %w)", op, retryErr, err)
	}

	logReply(ctx, log, 2, started, text)

	return text, nil
}

func logReply(ctx context.Context, log *slog.Logger, attempt int, started time.Time, reply string) {
	log.InfoContext(ctx, "GigaChat replied",
		slog.Int("attempt", attempt),
		slog.Int64("took_ms", time.Since(started).Milliseconds()),
		slog.Int("reply_chars", len([]rune(reply))),
	)
}

type chatRequest struct {
	Model          string              `json:"model"`
	Messages       []Message           `json:"messages"`
	Temperature    float64             `json:"temperature"`
	MaxTokens      int                 `json:"max_tokens"`
	ResponseFormat *chatResponseFormat `json:"response_format,omitempty"`
}

type chatResponseFormat struct {
	Type   string          `json:"type"`
	Schema json.RawMessage `json:"schema"`
	Strict bool            `json:"strict"`
}

var assessmentResponseFormat = &chatResponseFormat{
	Type: "json_schema",
	Schema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"criteria":{"type":"array","items":{"type":"object","properties":{
				"id":{"type":"string"},"met":{"type":"boolean"},
				"evidence":{"type":"string"},"comment":{"type":"string"}
			},"required":["id","met","evidence","comment"]}},
			"what_worked":{"type":"string"},"what_to_try_next":{"type":"string"}
		},
		"required":["criteria","what_worked","what_to_try_next"]
	}`),
	Strict: true,
}

type chatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}

func (g *gigachat) complete(ctx context.Context, request Request) (string, error) {
	const op = "gigachat.complete"

	log := g.log.With(slog.String("op", op))

	token, err := g.accessToken(ctx)
	if err != nil {
		return "", fmt.Errorf("%s: authorize in gigachat: %w", op, err)
	}

	chat := chatRequest{
		Model:       g.cfg.Model,
		Messages:    request.Messages,
		Temperature: g.cfg.Temperature,
		MaxTokens:   g.cfg.MaxTokens,
	}
	if request.Kind == KindAssessment {
		chat.ResponseFormat = assessmentResponseFormat
	}

	body, err := json.Marshal(chat)
	if err != nil {
		log.ErrorContext(ctx, "encode gigachat request", slog.Any("error", err))

		return "", fmt.Errorf("%s: encode gigachat request: %w", op, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		log.ErrorContext(ctx, "build gigachat request", slog.Any("error", err))

		return "", fmt.Errorf("%s: build gigachat request: %w", op, err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := g.http.Do(req)
	if err != nil {
		log.ErrorContext(ctx, "call gigachat", slog.Any("error", err))

		return "", fmt.Errorf("%s: call gigachat: %w", op, err)
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.WarnContext(ctx, "close gigachat response body", slog.Any("error", err))
		}
	}()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		log.ErrorContext(ctx, "read gigachat response", slog.Any("error", err))

		return "", fmt.Errorf("%s: read gigachat response: %w", op, err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		g.resetToken()
		log.WarnContext(ctx, "gigachat rejected the access token")

		return "", fmt.Errorf("%s: %w: %s", op, errUnauthorized, snippet(payload))
	}

	if resp.StatusCode != http.StatusOK {
		log.ErrorContext(ctx, "gigachat returned an unexpected status",
			slog.Int("status", resp.StatusCode),
			slog.String("body", snippet(payload)),
		)

		return "", fmt.Errorf("%s: gigachat returned %d: %s", op, resp.StatusCode, snippet(payload))
	}

	var parsed chatResponse

	if err := json.Unmarshal(payload, &parsed); err != nil {
		log.ErrorContext(ctx, "decode gigachat response", slog.Any("error", err))

		return "", fmt.Errorf("%s: decode gigachat response: %w", op, err)
	}

	if len(parsed.Choices) == 0 {
		log.ErrorContext(ctx, "gigachat returned no choices")

		return "", fmt.Errorf("%s: %w", op, errNoChoices)
	}

	text := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if text == "" {
		log.ErrorContext(ctx, "gigachat returned an empty reply")

		return "", fmt.Errorf("%s: %w", op, errEmptyReply)
	}

	return text, nil
}

func (g *gigachat) accessToken(ctx context.Context) (string, error) {
	const op = "gigachat.accessToken"

	log := g.log.With(slog.String("op", op))

	g.mu.Lock()
	defer g.mu.Unlock()

	if g.token != "" && time.Now().Before(g.expiry) {
		return g.token, nil
	}

	form := url.Values{"scope": {g.cfg.Scope}}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.cfg.AuthURL, strings.NewReader(form.Encode()))
	if err != nil {
		log.ErrorContext(ctx, "build gigachat token request", slog.Any("error", err))

		return "", fmt.Errorf("%s: build gigachat token request: %w", op, err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("RqUID", uuid.New().String())
	req.Header.Set("Authorization", "Basic "+g.cfg.BasicCredentials())

	resp, err := g.http.Do(req)
	if err != nil {
		log.ErrorContext(ctx, "request gigachat token", slog.Any("error", err))

		return "", fmt.Errorf("%s: request gigachat token: %w", op, err)
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.WarnContext(ctx, "close gigachat token response body", slog.Any("error", err))
		}
	}()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		log.ErrorContext(ctx, "read gigachat token response", slog.Any("error", err))

		return "", fmt.Errorf("%s: read gigachat token response: %w", op, err)
	}

	if resp.StatusCode != http.StatusOK {
		log.ErrorContext(ctx, "gigachat token request failed",
			slog.Int("status", resp.StatusCode),
			slog.String("body", snippet(payload)),
		)

		return "", fmt.Errorf("%s: gigachat token request returned %d: %s", op, resp.StatusCode, snippet(payload))
	}

	var parsed struct {
		AccessToken string `json:"access_token"`
		ExpiresAt   int64  `json:"expires_at"`
	}

	if err := json.Unmarshal(payload, &parsed); err != nil {
		log.ErrorContext(ctx, "decode gigachat token response", slog.Any("error", err))

		return "", fmt.Errorf("%s: decode gigachat token response: %w", op, err)
	}

	if parsed.AccessToken == "" {
		log.ErrorContext(ctx, "gigachat token response contains no access_token")

		return "", fmt.Errorf("%s: %w", op, errNoAccessToken)
	}

	g.token = parsed.AccessToken
	g.expiry = time.Now().Add(25 * time.Minute)

	if parsed.ExpiresAt > 0 {
		g.expiry = time.UnixMilli(parsed.ExpiresAt).Add(-time.Minute)
	}

	log.DebugContext(ctx, "gigachat access token refreshed", slog.Time("expires_at", g.expiry))

	return g.token, nil
}

func (g *gigachat) resetToken() {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.token = ""
	g.expiry = time.Time{}
}
