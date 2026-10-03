package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientIPPrefersForwardedForBehindProxy(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		remoteAddr string
		forwarded  string
		want       string
	}{
		{name: "proxy forwards client", remoteAddr: "172.18.0.5:41234", forwarded: "203.0.113.7", want: "203.0.113.7"},
		{name: "chain keeps rightmost public", remoteAddr: "172.18.0.5:41234", forwarded: "198.51.100.9, 203.0.113.7, 10.0.0.2", want: "203.0.113.7"},
		{name: "spoofed header from public client ignored", remoteAddr: "203.0.113.7:41234", forwarded: "198.51.100.9", want: "203.0.113.7"},
		{name: "no header falls back to remote", remoteAddr: "172.18.0.5:41234", forwarded: "", want: "172.18.0.5"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/auth/login", nil)
			r.RemoteAddr = tc.remoteAddr

			if tc.forwarded != "" {
				r.Header.Set("X-Forwarded-For", tc.forwarded)
			}

			if got := clientIP(r); got != tc.want {
				t.Fatalf("clientIP() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLoginLimiterIsolatesClientsAndExpiresEntries(t *testing.T) {
	t.Parallel()

	now := time.Now()
	limiter := newLoginLimiter(now)

	for i := range loginRateMaxAttempts {
		if allowed, _ := limiter.allow("203.0.113.7", now); !allowed {
			t.Fatalf("attempt %d for the first client was rejected", i+1)
		}
	}

	allowed, retryAfter := limiter.allow("203.0.113.7", now)
	if allowed {
		t.Fatal("attempt over the limit was allowed")
	}

	if retryAfter != loginRateWindow {
		t.Fatalf("retryAfter = %v, want %v", retryAfter, loginRateWindow)
	}

	if allowed, _ := limiter.allow("198.51.100.9", now); !allowed {
		t.Fatal("a different client was rejected by another client's attempts")
	}

	later := now.Add(loginRateWindow)
	if allowed, _ := limiter.allow("203.0.113.7", later); !allowed {
		t.Fatal("attempt after the window slid past every timestamp was rejected")
	}

	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	if _, stale := limiter.attempts["198.51.100.9"]; stale {
		t.Fatal("expired entry was not swept")
	}
}

func TestLoginLimiterFreesSlotsGradually(t *testing.T) {
	t.Parallel()

	const key = "203.0.113.7"

	half := loginRateMaxAttempts / 2
	now := time.Now()
	limiter := newLoginLimiter(now)

	for range half {
		if allowed, _ := limiter.allow(key, now); !allowed {
			t.Fatal("early attempt was rejected")
		}
	}

	midpoint := now.Add(loginRateWindow / 2)

	for range half {
		if allowed, _ := limiter.allow(key, midpoint); !allowed {
			t.Fatal("attempt within the limit was rejected")
		}
	}

	allowed, retryAfter := limiter.allow(key, midpoint)
	if allowed {
		t.Fatal("attempt over the limit was allowed")
	}

	if want := loginRateWindow / 2; retryAfter != want {
		t.Fatalf("retryAfter = %v, want %v", retryAfter, want)
	}

	afterFirstHalf := now.Add(loginRateWindow)

	for i := range half {
		if allowed, _ := limiter.allow(key, afterFirstHalf); !allowed {
			t.Fatalf("slot %d freed by the sliding window was not reused", i+1)
		}
	}

	if allowed, _ := limiter.allow(key, afterFirstHalf); allowed {
		t.Fatal("more slots were freed than the window had released")
	}
}

func TestRetryAfterSecondsRoundsUp(t *testing.T) {
	t.Parallel()

	cases := []struct {
		wait time.Duration
		want int
	}{
		{wait: loginRateWindow, want: 60},
		{wait: 20 * time.Second, want: 20},
		{wait: 1500 * time.Millisecond, want: 2},
		{wait: time.Millisecond, want: 1},
		{wait: 0, want: 1},
		{wait: -time.Second, want: 1},
	}

	for _, tc := range cases {
		if got := retryAfterSeconds(tc.wait); got != tc.want {
			t.Errorf("retryAfterSeconds(%v) = %d, want %d", tc.wait, got, tc.want)
		}
	}
}
