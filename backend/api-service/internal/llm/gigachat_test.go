package llm

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestGigaChatAssessmentRequestsStructuredJSON(t *testing.T) {
	var requests []map[string]json.RawMessage

	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body map[string]json.RawMessage

		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}

		requests = append(requests, body)

		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"role":"assistant","content":"{}"}}]}`))}, nil
	})}

	g := &gigachat{
		cfg:    config.GigaChatConfig{BaseURL: "https://example.test", Model: "GigaChat", RequestTimeout: time.Second},
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		http:   client,
		token:  "test-token",
		expiry: time.Now().Add(time.Hour),
	}

	for _, kind := range []string{KindAssessment, KindDialogue} {
		if _, err := g.Complete(context.Background(), Request{Kind: kind, Messages: []Message{{Role: RoleUser, Content: "test"}}}); err != nil {
			t.Fatalf("complete %s: %v", kind, err)
		}
	}

	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}

	var format struct {
		Type   string          `json:"type"`
		Schema json.RawMessage `json:"schema"`
		Strict bool            `json:"strict"`
	}

	if err := json.Unmarshal(requests[0]["response_format"], &format); err != nil {
		t.Fatalf("decode response_format: %v", err)
	}

	if format.Type != "json_schema" || !format.Strict || !json.Valid(format.Schema) {
		t.Fatalf("invalid assessment response format: %+v", format)
	}

	if _, ok := requests[1]["response_format"]; ok {
		t.Fatal("dialogue request unexpectedly has response_format")
	}
}
