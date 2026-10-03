package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSRejectsForeignMutationOrigin(t *testing.T) {
	handler := CORS([]string{"https://app.example.test"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, tc := range []struct {
		origin string
		want   int
	}{
		{"https://evil.example.test", http.StatusForbidden},
		{"https://app.example.test", http.StatusNoContent},
		{"https://api.example.test", http.StatusNoContent},
	} {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "https://api.example.test/action", nil)
		req.Header.Set("Origin", tc.origin)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("origin %s: status %d, want %d", tc.origin, rec.Code, tc.want)
		}
	}
}
