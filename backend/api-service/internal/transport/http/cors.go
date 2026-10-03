package http

import (
	"net/http"
	"net/url"
	"slices"
)

const (
	corsAllowAllOrigins = "*"
	corsAllowedMethods  = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
	corsAllowedHeaders  = "Content-Type, Authorization"
	corsMaxAge          = "300"
)

func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowAll := slices.Contains(allowedOrigins, corsAllowAllOrigins)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Vary", "Origin")

			origin := r.Header.Get("Origin")
			allowed := allowAll || slices.Contains(allowedOrigins, origin) || sameHostOrigin(origin, r.Host)
			if origin != "" && r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions && !allowed {
				writeError(w, http.StatusForbidden, "origin_forbidden", "Origin is not allowed")

				return
			}

			if origin != "" {
				switch {
				case allowAll:
					w.Header().Set("Access-Control-Allow-Origin", corsAllowAllOrigins)

				case allowed:
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
			}

			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", corsAllowedMethods)
				w.Header().Set("Access-Control-Allow-Headers", corsAllowedHeaders)
				w.Header().Set("Access-Control-Max-Age", corsMaxAge)
				w.WriteHeader(http.StatusNoContent)

				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func sameHostOrigin(origin, host string) bool {
	u, err := url.Parse(origin)

	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host == host
}
