package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func decodeRecord(t *testing.T, raw []byte) map[string]any {
	t.Helper()

	var record map[string]any

	if err := json.Unmarshal(raw, &record); err != nil {
		t.Fatalf("decode log record: %v", err)
	}

	return record
}

func TestRequestLoggerLogsHandledRequest(t *testing.T) {
	var buf bytes.Buffer

	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	var contextRequestID string

	handler := RequestLogger(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contextRequestID = RequestID(r.Context())

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/companies/c1/sessions", nil))

	if contextRequestID == "" {
		t.Fatal("request id was not put into the request context")
	}

	if got := recorder.Header().Get(RequestIDHeader); got != contextRequestID {
		t.Errorf("response header %s = %q, want %q", RequestIDHeader, got, contextRequestID)
	}

	record := decodeRecord(t, buf.Bytes())

	if record["level"] != "INFO" {
		t.Errorf("level = %v, want INFO", record["level"])
	}

	if record["msg"] != "request handled" {
		t.Errorf("msg = %v, want request handled", record["msg"])
	}

	if record["status"] != float64(http.StatusCreated) {
		t.Errorf("status = %v, want %d", record["status"], http.StatusCreated)
	}

	if record["method"] != http.MethodPost {
		t.Errorf("method = %v, want %s", record["method"], http.MethodPost)
	}

	if record["path"] != "/api/v1/companies/c1/sessions" {
		t.Errorf("path = %v", record["path"])
	}
}

func TestRequestLoggerReusesIncomingRequestID(t *testing.T) {
	var buf bytes.Buffer

	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	const incoming = "11111111-2222-3333-4444-555555555555"

	var contextRequestID string

	handler := RequestLogger(logger)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		contextRequestID = RequestID(r.Context())
	}))

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health/live", nil)
	request.Header.Set(RequestIDHeader, incoming)

	handler.ServeHTTP(httptest.NewRecorder(), request)

	if contextRequestID != incoming {
		t.Errorf("request id = %q, want %q", contextRequestID, incoming)
	}
}

func TestRequestLoggerLevelByStatus(t *testing.T) {
	tests := []struct {
		name   string
		status int
		level  string
		msg    string
	}{
		{"client error", http.StatusBadRequest, "WARN", "request rejected"},
		{"server error", http.StatusInternalServerError, "ERROR", "request failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			logger := slog.New(slog.NewJSONHandler(&buf, nil))

			handler := RequestLogger(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))

			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/me", nil))

			record := decodeRecord(t, buf.Bytes())

			if record["level"] != tt.level {
				t.Errorf("level = %v, want %v", record["level"], tt.level)
			}

			if record["msg"] != tt.msg {
				t.Errorf("msg = %v, want %v", record["msg"], tt.msg)
			}
		})
	}
}

func TestRemoteIPPrefersForwardedFor(t *testing.T) {
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health/live", nil)
	request.RemoteAddr = "10.0.0.1:34567"
	request.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.1")

	if got := remoteIP(request); got != "203.0.113.7" {
		t.Errorf("remoteIP() = %q, want 203.0.113.7", got)
	}

	request.Header.Del("X-Forwarded-For")

	if got := remoteIP(request); got != "10.0.0.1" {
		t.Errorf("remoteIP() = %q, want 10.0.0.1", got)
	}
}

func TestRequestIDHandlerAddsRequestIDFromContext(t *testing.T) {
	var buf bytes.Buffer

	logger := slog.New(RequestIDHandler(slog.NewJSONHandler(&buf, nil)))

	logger.ErrorContext(WithRequestID(t.Context(), "req-42"), "boom", slog.String("op", "negotiationService.Hint"))

	record := decodeRecord(t, buf.Bytes())

	if record["request_id"] != "req-42" {
		t.Errorf("request_id = %v, want req-42", record["request_id"])
	}

	if record["op"] != "negotiationService.Hint" {
		t.Errorf("op = %v, want negotiationService.Hint", record["op"])
	}
}

func TestRequestIDHandlerWithoutRequestID(t *testing.T) {
	var buf bytes.Buffer

	logger := slog.New(RequestIDHandler(slog.NewJSONHandler(&buf, nil)))

	logger.InfoContext(context.Background(), "plain")

	if strings.Contains(buf.String(), "request_id") {
		t.Errorf("unexpected request_id in %q", buf.String())
	}
}

func TestRequestIDHandlerKeepsAttrs(t *testing.T) {
	var buf bytes.Buffer

	logger := slog.New(RequestIDHandler(slog.NewJSONHandler(&buf, nil))).
		With(slog.String("op", "authService.Login"))

	logger.WarnContext(WithRequestID(t.Context(), "req-7"), "rejected")

	record := decodeRecord(t, buf.Bytes())

	if record["op"] != "authService.Login" {
		t.Errorf("op = %v, want authService.Login", record["op"])
	}

	if record["request_id"] != "req-7" {
		t.Errorf("request_id = %v, want req-7", record["request_id"])
	}
}

func TestRequestIDHandlerNestsRequestIDIntoGroup(t *testing.T) {
	var buf bytes.Buffer

	logger := slog.New(RequestIDHandler(slog.NewJSONHandler(&buf, nil))).WithGroup("request")

	logger.WarnContext(WithRequestID(t.Context(), "req-7"), "rejected", slog.String("path", "/api/v1/auth/login"))

	record := decodeRecord(t, buf.Bytes())

	group, ok := record["request"].(map[string]any)
	if !ok {
		t.Fatalf("record has no request group: %v", record)
	}

	if group["request_id"] != "req-7" {
		t.Errorf("request.request_id = %v, want req-7", group["request_id"])
	}
}
