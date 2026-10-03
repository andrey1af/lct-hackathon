package config

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

const (
	EnvLocal = "local"
	EnvDev   = "dev"
	EnvProd  = "prod"
)

type Config struct {
	Env         string `env:"ENV"`
	HTTP        HTTPConfig
	DB          DBConfig
	SETTINGS    SettingsConfig
	GigaChat    GigaChatConfig
	Negotiation NegotiationConfig
}

type SettingsConfig struct {
	AccessTokenTTL        time.Duration `env:"ACCESS_TOKEN_TTL" env-default:"15m"`
	RefreshTokenTTL       time.Duration `env:"REFRESH_TOKEN_TTL" env-default:"720h"`
	RefreshCookieName     string        `env:"REFRESH_COOKIE_NAME" env-default:"refresh_token"`
	RefreshCookieInsecure bool          `env:"REFRESH_COOKIE_INSECURE" env-default:"false"`
	JWTSecret             string        `env:"JWT_SECRET" env-required:"true"`
}

type HTTPConfig struct {
	Port               int           `env:"HTTP_PORT" env-default:"8090"`
	ReadHeaderTimeout  time.Duration `env:"HTTP_READ_HEADER_TIMEOUT" env-default:"5s"`
	ShutdownTimeout    time.Duration `env:"HTTP_SHUTDOWN_TIMEOUT" env-default:"10s"`
	HealthTimeout      time.Duration `env:"HEALTHCHECK_TIMEOUT" env-default:"2s"`
	CORSAllowedOrigins []string      `env:"CORS_ALLOWED_ORIGINS" env-separator:","`
	OpenAPISpecPath    string        `env:"OPENAPI_SPEC_PATH" env-default:"../../api/openapi.yaml"`
}

type DBConfig struct {
	DSN             string        `env:"-"`
	Host            string        `env:"DB_HOST" env-default:"localhost"`
	Port            int           `env:"DB_PORT" env-default:"5432"`
	User            string        `env:"DB_USER" env-default:"postgres"`
	Password        string        `env:"DB_PASSWORD"`
	Name            string        `env:"DB_NAME" env-default:"negotiation_simulator"`
	SSLMode         string        `env:"DB_SSLMODE" env-default:"disable"`
	MaxOpenConns    int           `env:"DB_MAX_OPEN_CONNS" env-default:"25"`
	MaxIdleConns    int           `env:"DB_MAX_IDLE_CONNS" env-default:"25"`
	ConnMaxLifetime time.Duration `env:"DB_CONN_MAX_LIFETIME" env-default:"5m"`
}

type GigaChatConfig struct {
	AuthKey        string        `env:"GIGACHAT_AUTH_KEY"`
	ClientID       string        `env:"GIGACHAT_CLIENT_ID"`
	ClientSecret   string        `env:"GIGACHAT_CLIENT_SECRET"`
	Scope          string        `env:"GIGACHAT_SCOPE" env-default:"GIGACHAT_API_PERS"`
	Model          string        `env:"GIGACHAT_MODEL"`
	AuthURL        string        `env:"GIGACHAT_AUTH_URL" env-default:"https://ngw.devices.sberbank.ru:9443/api/v2/oauth"`
	BaseURL        string        `env:"GIGACHAT_BASE_URL" env-default:"https://gigachat.devices.sberbank.ru/api/v1"`
	RequestTimeout time.Duration `env:"GIGACHAT_REQUEST_TIMEOUT" env-default:"30s"`
	MaxTokens      int           `env:"GIGACHAT_MAX_TOKENS" env-default:"1024"`
	Temperature    float64       `env:"GIGACHAT_TEMPERATURE" env-default:"0.7"`
}

func (c GigaChatConfig) BasicCredentials() string {
	if c.AuthKey != "" {
		return c.AuthKey
	}

	if c.ClientID == "" || c.ClientSecret == "" {
		return ""
	}

	return base64.StdEncoding.EncodeToString([]byte(c.ClientID + ":" + c.ClientSecret))
}

type NegotiationConfig struct {
	DefaultCaseKey string `env:"NEGOTIATION_DEFAULT_CASE_KEY" env-default:"practice-procurement"`
}

func buildDSN(c DBConfig) string {
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.User, c.Password),
		Host:   net.JoinHostPort(c.Host, strconv.Itoa(c.Port)),
		Path:   "/" + c.Name,
	}

	query := url.Values{}
	query.Set("sslmode", c.SSLMode)
	u.RawQuery = query.Encode()

	return u.String()
}

func Load() (Config, error) {
	var cfg Config

	if err := cleanenv.ReadEnv(&cfg); err != nil {
		return Config{}, fmt.Errorf("read env: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	cfg.DB.DSN = buildDSN(cfg.DB)

	return cfg, nil
}

func (c Config) validate() error {
	if c.HTTP.Port < 1 || c.HTTP.Port > 65535 {
		return fmt.Errorf("HTTP_PORT must be a valid TCP port")
	}

	if c.HTTP.ReadHeaderTimeout <= 0 {
		return fmt.Errorf("HTTP_READ_HEADER_TIMEOUT must be positive")
	}

	if c.HTTP.ShutdownTimeout <= 0 {
		return fmt.Errorf("HTTP_SHUTDOWN_TIMEOUT must be positive")
	}

	if c.HTTP.HealthTimeout <= 0 {
		return fmt.Errorf("HEALTHCHECK_TIMEOUT must be positive")
	}

	if c.HTTP.OpenAPISpecPath == "" {
		return fmt.Errorf("OPENAPI_SPEC_PATH must not be empty")
	}

	if c.DB.Port < 1 || c.DB.Port > 65535 {
		return fmt.Errorf("DB_PORT must be a valid TCP port")
	}

	if c.DB.Password == "" {
		return fmt.Errorf("DB_PASSWORD must not be empty")
	}

	if c.DB.MaxOpenConns < 1 {
		return fmt.Errorf("DB_MAX_OPEN_CONNS must be positive")
	}

	if c.DB.MaxIdleConns < 1 {
		return fmt.Errorf("DB_MAX_IDLE_CONNS must be positive")
	}

	if c.DB.ConnMaxLifetime <= 0 {
		return fmt.Errorf("DB_CONN_MAX_LIFETIME must be positive")
	}

	if c.GigaChat.RequestTimeout <= 0 {
		return fmt.Errorf("GIGACHAT_REQUEST_TIMEOUT must be positive")
	}

	if c.GigaChat.MaxTokens < 1 {
		return fmt.Errorf("GIGACHAT_MAX_TOKENS must be positive")
	}

	if c.GigaChat.BasicCredentials() != "" && c.GigaChat.Model == "" {
		return fmt.Errorf("GIGACHAT_MODEL must not be empty when GigaChat credentials are configured")
	}

	if c.Negotiation.DefaultCaseKey == "" {
		return fmt.Errorf("NEGOTIATION_DEFAULT_CASE_KEY must not be empty")
	}

	switch c.Env {
	case EnvLocal, EnvDev, EnvProd:
	default:
		return fmt.Errorf("ENV must be one of %s, %s, %s", EnvLocal, EnvDev, EnvProd)
	}

	return nil
}
