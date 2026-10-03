package http

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	loginRateMaxAttempts = 60
	loginRateWindow      = time.Minute
)

type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	swept    time.Time
}

func newLoginLimiter(now time.Time) *loginLimiter {
	return &loginLimiter{attempts: make(map[string][]time.Time), swept: now}
}

func (l *loginLimiter) allow(key string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if now.Sub(l.swept) >= loginRateWindow {
		l.sweep(now)

		l.swept = now
	}

	stamps := withinWindow(l.attempts[key], now)
	if len(stamps) >= loginRateMaxAttempts {
		l.attempts[key] = stamps

		return false, loginRateWindow - now.Sub(stamps[0])
	}

	l.attempts[key] = append(stamps, now)

	return true, 0
}

func (l *loginLimiter) sweep(now time.Time) {
	for key, stamps := range l.attempts {
		remaining := withinWindow(stamps, now)
		if len(remaining) == 0 {
			delete(l.attempts, key)

			continue
		}

		l.attempts[key] = remaining
	}
}

func withinWindow(stamps []time.Time, now time.Time) []time.Time {
	cutoff := now.Add(-loginRateWindow)

	index := 0

	for index < len(stamps) && !stamps[index].After(cutoff) {
		index++
	}

	return stamps[index:]
}

func loginRateLimit(next http.Handler) http.Handler {
	limiter := newLoginLimiter(time.Now())

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowed, retryAfter := limiter.allow(clientIP(r), time.Now())
		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(retryAfter)))
			writeError(w, http.StatusTooManyRequests, "rate_limited", "Try again later")

			return
		}

		next.ServeHTTP(w, r)
	})
}

func retryAfterSeconds(wait time.Duration) int {
	seconds := int((wait + time.Second - 1) / time.Second)
	if seconds < 1 {
		return 1
	}

	return seconds
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}

	if !trustedProxy(net.ParseIP(host)) {
		return host
	}

	forwarded := strings.Split(r.Header.Get("X-Forwarded-For"), ",")

	for i := len(forwarded) - 1; i >= 0; i-- {
		candidate := net.ParseIP(strings.TrimSpace(forwarded[i]))
		if candidate != nil && !trustedProxy(candidate) {
			return candidate.String()
		}
	}

	return host
}

func trustedProxy(ip net.IP) bool {
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast())
}
