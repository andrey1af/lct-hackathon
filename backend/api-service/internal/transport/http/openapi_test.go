package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRouter_OpenAPIAndSwagger(t *testing.T) {
	spec := []byte("openapi: 3.0.3\ninfo:\n  title: test\n")
	pass := func(next http.Handler) http.Handler { return next }
	router := NewRouter(NewHealthHandler(fakePinger{}, time.Second), []string{"*"}, spec, nil, nil, pass, pass, NewUserHandler(nil), nil)

	tests := []struct {
		name       string
		path       string
		wantSubstr string
	}{
		{name: "openapi spec", path: "/openapi.yaml", wantSubstr: "openapi: 3.0.3"},
		{name: "swagger ui", path: "/swagger", wantSubstr: "swagger-ui"},
		{name: "swagger ui trailing slash", path: "/swagger/", wantSubstr: "swagger-ui"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, tt.path, nil)

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}

			if !strings.Contains(rec.Body.String(), tt.wantSubstr) {
				t.Errorf("body does not contain %q", tt.wantSubstr)
			}
		})
	}
}

func TestSwaggerUI_SubresourceIntegrityAndCSP(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/swagger", nil)

	swaggerUIHandler(rec, req)

	body := rec.Body.String()

	for _, want := range []string{
		`integrity="` + swaggerUICSSSRI + `"`,
		`integrity="` + swaggerUIBundleSRI + `"`,
		`crossorigin="anonymous"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("swagger page does not contain %q", want)
		}
	}

	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("Content-Security-Policy header is empty")
	}

	if !strings.Contains(csp, inlineScriptHash(swaggerInitScript)) {
		t.Errorf("CSP %q does not allow the inline init script", csp)
	}

	if !strings.Contains(body, swaggerInitScript) {
		t.Error("swagger page does not contain the hashed init script verbatim")
	}

	if !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Errorf("CSP %q does not forbid framing", csp)
	}
}
