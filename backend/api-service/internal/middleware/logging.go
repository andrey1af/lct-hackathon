package middleware

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
	"uuid"
)

const RequestIDHeader = "X-Request-Id"

type requestIDKey struct{}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

func RequestID(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDKey{}).(string)

	return requestID
}

func RequestIDHandler(handler slog.Handler) slog.Handler {
	return requestIDHandler{Handler: handler}
}

type requestIDHandler struct {
	slog.Handler
}

func (h requestIDHandler) Handle(ctx context.Context, record slog.Record) error {
	if requestID := RequestID(ctx); requestID != "" {
		record.AddAttrs(slog.String("request_id", requestID))
	}

	return h.Handler.Handle(ctx, record)
}

func (h requestIDHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return requestIDHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h requestIDHandler) WithGroup(name string) slog.Handler {
	return requestIDHandler{Handler: h.Handler.WithGroup(name)}
}

type responseRecorder struct {
	http.ResponseWriter
	status  int
	written int
}

func (r *responseRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.written += n

	return n, err
}

func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get(RequestIDHeader)
			if requestID == "" {
				requestID = uuid.New().String()
			}

			w.Header().Set(RequestIDHeader, requestID)

			ctx := WithRequestID(r.Context(), requestID)
			recorder := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
			started := time.Now()

			next.ServeHTTP(recorder, r.WithContext(ctx))

			attrs := []any{
				"method", r.Method,
				"path", r.URL.Path,
				"status", recorder.status,
				"duration_ms", time.Since(started).Milliseconds(),
				"bytes", recorder.written,
				"remote_ip", remoteIP(r),
			}

			switch {
			case recorder.status >= http.StatusInternalServerError:
				logger.ErrorContext(ctx, "request failed", attrs...)

			case recorder.status >= http.StatusBadRequest:
				logger.WarnContext(ctx, "request rejected", attrs...)

			default:
				logger.InfoContext(ctx, "request handled", attrs...)
			}
		})
	}
}

func remoteIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		first, _, _ := strings.Cut(forwarded, ",")

		return strings.TrimSpace(first)
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return host
}
