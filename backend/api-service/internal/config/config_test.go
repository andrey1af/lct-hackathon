package config

import (
	"os"
	"reflect"
	"testing"
	"time"
)

const (
	envHTTPPort              = "HTTP_PORT"
	envHTTPReadHeaderTimeout = "HTTP_READ_HEADER_TIMEOUT"
	envHTTPShutdownTimeout   = "HTTP_SHUTDOWN_TIMEOUT"
	envHealthcheckTimeout    = "HEALTHCHECK_TIMEOUT"
	envCORSAllowedOrigins    = "CORS_ALLOWED_ORIGINS"
	envOpenAPISpecPath       = "OPENAPI_SPEC_PATH"

	envDBHost            = "DB_HOST"
	envDBPort            = "DB_PORT"
	envDBUser            = "DB_USER"
	envDBPassword        = "DB_PASSWORD"
	envDBName            = "DB_NAME"
	envDBSSLMode         = "DB_SSLMODE"
	envDBMaxOpenConns    = "DB_MAX_OPEN_CONNS"
	envDBMaxIdleConns    = "DB_MAX_IDLE_CONNS"
	envDBConnMaxLifetime = "DB_CONN_MAX_LIFETIME"

	envGigaChatAuthKey        = "GIGACHAT_AUTH_KEY"
	envGigaChatClientID       = "GIGACHAT_CLIENT_ID"
	envGigaChatClientSecret   = "GIGACHAT_CLIENT_SECRET"
	envGigaChatScope          = "GIGACHAT_SCOPE"
	envGigaChatModel          = "GIGACHAT_MODEL"
	envGigaChatAuthURL        = "GIGACHAT_AUTH_URL"
	envGigaChatBaseURL        = "GIGACHAT_BASE_URL"
	envGigaChatRequestTimeout = "GIGACHAT_REQUEST_TIMEOUT"
	envGigaChatMaxTokens      = "GIGACHAT_MAX_TOKENS"
	envGigaChatTemperature    = "GIGACHAT_TEMPERATURE"

	envNegotiationDefaultCaseKey = "NEGOTIATION_DEFAULT_CASE_KEY"

	envJWTSecret       = "JWT_SECRET"
	envAccessTokenTTL  = "ACCESS_TOKEN_TTL"
	envRefreshTokenTTL = "REFRESH_TOKEN_TTL"

	envEnv = "ENV"
)

var allEnvVars = []string{
	envHTTPPort,
	envHTTPReadHeaderTimeout,
	envHTTPShutdownTimeout,
	envHealthcheckTimeout,
	envCORSAllowedOrigins,
	envOpenAPISpecPath,
	envDBHost,
	envDBPort,
	envDBUser,
	envDBPassword,
	envDBName,
	envDBSSLMode,
	envDBMaxOpenConns,
	envDBMaxIdleConns,
	envDBConnMaxLifetime,
	envGigaChatAuthKey,
	envGigaChatClientID,
	envGigaChatClientSecret,
	envGigaChatScope,
	envGigaChatModel,
	envGigaChatAuthURL,
	envGigaChatBaseURL,
	envGigaChatRequestTimeout,
	envGigaChatMaxTokens,
	envGigaChatTemperature,
	envNegotiationDefaultCaseKey,
	envJWTSecret,
	envAccessTokenTTL,
	envRefreshTokenTTL,
	envEnv,
}

const testDBPassword = "test-password"

func setDBPassword(t *testing.T) {
	t.Helper()

	t.Setenv(envDBPassword, testDBPassword)
}

const testJWTSecret = "test-jwt-secret"

func setJWTSecret(t *testing.T) {
	t.Helper()

	t.Setenv(envJWTSecret, testJWTSecret)
}

func clearEnv(t *testing.T) {
	t.Helper()

	for _, name := range allEnvVars {
		prev, wasSet := os.LookupEnv(name)
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("unsetenv %s: %v", name, err)
		}

		t.Cleanup(func() {
			if wasSet {
				_ = os.Setenv(name, prev)
			} else {
				_ = os.Unsetenv(name)
			}
		})
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	clearEnv(t)

	t.Setenv(envHTTPPort, "9090")
	t.Setenv(envHTTPReadHeaderTimeout, "1s")
	t.Setenv(envHTTPShutdownTimeout, "3s")
	t.Setenv(envHealthcheckTimeout, "500ms")
	t.Setenv(envCORSAllowedOrigins, "http://localhost:5173,http://localhost:3000")
	t.Setenv(envOpenAPISpecPath, "/app/openapi.yaml")
	t.Setenv(envDBHost, "db.internal")
	t.Setenv(envDBPort, "5433")
	t.Setenv(envDBUser, "app")
	t.Setenv(envDBPassword, "secret")
	t.Setenv(envDBName, "app_db")
	t.Setenv(envDBSSLMode, "require")
	t.Setenv(envDBMaxOpenConns, "10")
	t.Setenv(envDBMaxIdleConns, "5")
	t.Setenv(envDBConnMaxLifetime, "1m")
	t.Setenv(envGigaChatAuthKey, "auth-key")
	t.Setenv(envGigaChatScope, "GIGACHAT_API_CORP")
	t.Setenv(envGigaChatModel, "GigaChat-Max")
	t.Setenv(envGigaChatAuthURL, "https://auth.local/oauth")
	t.Setenv(envGigaChatBaseURL, "https://llm.local/api/v1")
	t.Setenv(envGigaChatRequestTimeout, "5s")
	t.Setenv(envGigaChatMaxTokens, "256")
	t.Setenv(envGigaChatTemperature, "0.2")
	t.Setenv(envNegotiationDefaultCaseKey, "practice-sales")
	t.Setenv(envJWTSecret, "override-secret")
	t.Setenv(envAccessTokenTTL, "30m")
	t.Setenv(envRefreshTokenTTL, "24h")
	t.Setenv(envEnv, "prod")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	want := Config{
		Env: EnvProd,
		HTTP: HTTPConfig{
			Port:               9090,
			ReadHeaderTimeout:  time.Second,
			ShutdownTimeout:    3 * time.Second,
			HealthTimeout:      500 * time.Millisecond,
			CORSAllowedOrigins: []string{"http://localhost:5173", "http://localhost:3000"},
			OpenAPISpecPath:    "/app/openapi.yaml",
		},
		DB: DBConfig{
			Host:            "db.internal",
			Port:            5433,
			User:            "app",
			Password:        "secret",
			Name:            "app_db",
			SSLMode:         "require",
			MaxOpenConns:    10,
			MaxIdleConns:    5,
			ConnMaxLifetime: time.Minute,
			DSN:             "postgres://app:secret@db.internal:5433/app_db?sslmode=require",
		},
		GigaChat: GigaChatConfig{
			AuthKey:        "auth-key",
			Scope:          "GIGACHAT_API_CORP",
			Model:          "GigaChat-Max",
			AuthURL:        "https://auth.local/oauth",
			BaseURL:        "https://llm.local/api/v1",
			RequestTimeout: 5 * time.Second,
			MaxTokens:      256,
			Temperature:    0.2,
		},
		Negotiation: NegotiationConfig{DefaultCaseKey: "practice-sales"},
		SETTINGS: SettingsConfig{
			RefreshCookieName: "refresh_token",
			AccessTokenTTL:    30 * time.Minute,
			RefreshTokenTTL:   24 * time.Hour,
			JWTSecret:         "override-secret",
		},
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("Load() = %+v, want %+v", cfg, want)
	}
}

func TestLoad_MissingDBPassword(t *testing.T) {
	tests := []struct {
		name   string
		setEnv func(t *testing.T)
	}{
		{"unset", func(*testing.T) {}},
		{"empty", func(t *testing.T) { t.Setenv(envDBPassword, "") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			setJWTSecret(t)
			tt.setEnv(t)

			if _, err := Load(); err == nil {
				t.Fatalf("Load() with %s %s: expected error, got nil", envDBPassword, tt.name)
			}
		})
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"not a number", "abc"},
		{"zero", "0"},
		{"negative", "-1"},
		{"above max", "70000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			setDBPassword(t)
			setJWTSecret(t)
			t.Setenv(envHTTPPort, tt.value)

			if _, err := Load(); err == nil {
				t.Fatalf("Load() with %s=%q: expected error, got nil", envHTTPPort, tt.value)
			}
		})
	}
}

func TestLoad_InvalidDBConfig(t *testing.T) {
	tests := []struct {
		name   string
		envVar string
		value  string
	}{
		{"db port not a number", envDBPort, "abc"},
		{"db port zero", envDBPort, "0"},
		{"db port above max", envDBPort, "70000"},
		{"db max open conns zero", envDBMaxOpenConns, "0"},
		{"db max open conns negative", envDBMaxOpenConns, "-1"},
		{"db max idle conns zero", envDBMaxIdleConns, "0"},
		{"db max idle conns negative", envDBMaxIdleConns, "-1"},
		{"db conn max lifetime not a duration", envDBConnMaxLifetime, "oops"},
		{"db conn max lifetime negative", envDBConnMaxLifetime, "-1s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			setDBPassword(t)
			setJWTSecret(t)
			t.Setenv(tt.envVar, tt.value)

			if _, err := Load(); err == nil {
				t.Fatalf("Load() with %s=%q: expected error, got nil", tt.envVar, tt.value)
			}
		})
	}
}

func TestLoad_InvalidDurations(t *testing.T) {
	tests := []struct {
		name   string
		envVar string
		value  string
	}{
		{"openapi spec path empty", envOpenAPISpecPath, ""},
		{"read header timeout not a duration", envHTTPReadHeaderTimeout, "oops"},
		{"read header timeout zero", envHTTPReadHeaderTimeout, "0s"},
		{"read header timeout negative", envHTTPReadHeaderTimeout, "-1s"},
		{"shutdown timeout not a duration", envHTTPShutdownTimeout, "oops"},
		{"shutdown timeout negative", envHTTPShutdownTimeout, "-5s"},
		{"healthcheck timeout not a duration", envHealthcheckTimeout, "oops"},
		{"healthcheck timeout negative", envHealthcheckTimeout, "-2s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			setDBPassword(t)
			setJWTSecret(t)
			t.Setenv(tt.envVar, tt.value)

			if _, err := Load(); err == nil {
				t.Fatalf("Load() with %s=%q: expected error, got nil", tt.envVar, tt.value)
			}
		})
	}
}

func TestBuildDSN(t *testing.T) {
	cfg := DBConfig{
		Host:     "db.internal",
		Port:     5433,
		User:     "app",
		Password: "secret",
		Name:     "app_db",
		SSLMode:  "require",
	}

	want := "postgres://app:secret@db.internal:5433/app_db?sslmode=require"
	if got := buildDSN(cfg); got != want {
		t.Errorf("buildDSN() = %q, want %q", got, want)
	}
}
