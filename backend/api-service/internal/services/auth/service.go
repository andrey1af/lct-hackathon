package auth

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"uuid"

	"gorm.io/gorm"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/config"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/model"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/repository"
)

const (
	minPasswordLen = 8
	maxPasswordLen = 72
)

type AuthService struct {
	userRepo  *repository.UserRepository
	tokenRepo *repository.TokenRepository
	cfg       config.Config
	db        *gorm.DB
	log       *slog.Logger
}

func NewAuthService(
	usRepo *repository.UserRepository,
	tokenRepo *repository.TokenRepository,
	cfg config.Config,
	db *gorm.DB,
	log *slog.Logger,
) *AuthService {
	return &AuthService{
		userRepo:  usRepo,
		tokenRepo: tokenRepo,
		cfg:       cfg,
		db:        db,
		log:       log,
	}
}

func (s *AuthService) CookieSettings() (refreshName string, accessTTL, refreshTTL time.Duration, secure bool) {
	name := s.cfg.SETTINGS.RefreshCookieName
	if name == "" {
		name = "refresh_token"
	}

	return name, s.cfg.SETTINGS.AccessTokenTTL, s.cfg.SETTINGS.RefreshTokenTTL, !s.cfg.SETTINGS.RefreshCookieInsecure
}

func (s *AuthService) Register(ctx context.Context, in RegisterRequest) (*model.JWT, error) {
	const op = "authService.Register"

	log := s.log.With(slog.String("op", op))

	email := normalizeEmail(in.Email)
	name := strings.TrimSpace(in.Name)

	if email == "" || name == "" {
		log.WarnContext(ctx, "registration rejected: empty email or name")

		return nil, fmt.Errorf("%s: %w", op, model.ErrInvalidInput)
	}

	if len(in.Password) < minPasswordLen || len(in.Password) > maxPasswordLen {
		log.WarnContext(ctx, "registration rejected: weak password")

		return nil, fmt.Errorf("%s: %w", op, model.ErrWeakPassword)
	}

	if _, err := s.userRepo.GetUserByEmail(ctx, email); err == nil {
		log.WarnContext(ctx, "registration rejected: email is already taken")

		return nil, fmt.Errorf("%s: %w", op, model.ErrEmailTaken)
	} else if !errors.Is(err, model.ErrUserNotFound) {
		log.ErrorContext(ctx, "look up user by email", slog.Any("error", err))

		return nil, fmt.Errorf("%s: %w", op, err)
	}

	hash, err := hashPassword(in.Password)
	if err != nil {
		log.ErrorContext(ctx, "hash password", slog.Any("error", err))

		return nil, fmt.Errorf("%s: %w", op, err)
	}

	u := &model.User{ID: uuid.New().String(), Email: email, PasswordHash: string(hash), Name: name}
	if err := s.userRepo.CreateUser(ctx, u); err != nil {
		log.ErrorContext(ctx, "create user", slog.Any("error", err))

		return nil, fmt.Errorf("%s: %w", op, err)
	}

	tokens, err := s.generateTokens(ctx, s.tokenRepo, u.ID)
	if err != nil {
		log.ErrorContext(ctx, "issue tokens", slog.String("user_id", u.ID), slog.Any("error", err))

		return nil, fmt.Errorf("%s: %w", op, err)
	}

	log.InfoContext(ctx, "user registered", slog.String("user_id", u.ID))

	return tokens, nil
}

func (s *AuthService) Login(ctx context.Context, in LoginRequest) (*model.JWT, error) {
	const op = "authService.Login"

	log := s.log.With(slog.String("op", op))

	user, err := s.userRepo.GetUserByEmail(ctx, normalizeEmail(in.Email))
	if err != nil {
		if errors.Is(err, model.ErrUserNotFound) {
			_ = checkPasswordHash(in.Password, decoyPasswordHash())

			log.WarnContext(ctx, "login rejected: unknown email")

			return nil, fmt.Errorf("%s: %w", op, model.ErrInvalidCredentials)
		}

		log.ErrorContext(ctx, "get user by email", slog.Any("error", err))

		return nil, fmt.Errorf("%s: %w", op, err)
	}

	if !checkPasswordHash(in.Password, user.PasswordHash) {
		log.WarnContext(ctx, "login rejected: wrong password", slog.String("user_id", user.ID))

		return nil, fmt.Errorf("%s: %w", op, model.ErrInvalidCredentials)
	}

	tokens, err := s.generateTokens(ctx, s.tokenRepo, user.ID)
	if err != nil {
		log.ErrorContext(ctx, "issue tokens", slog.String("user_id", user.ID), slog.Any("error", err))

		return nil, fmt.Errorf("%s: %w", op, err)
	}

	log.InfoContext(ctx, "user logged in", slog.String("user_id", user.ID))

	return tokens, nil
}

func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*model.JWT, error) {
	const op = "authService.Refresh"

	log := s.log.With(slog.String("op", op))

	refreshTokenHash := hashToken(refreshToken)

	var tokens *model.JWT

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tokenRepoTx := s.tokenRepo.WithTx(tx)

		storedToken, err := tokenRepoTx.ConsumeSession(ctx, refreshTokenHash)
		if err != nil {
			if errors.Is(err, model.ErrInvalidToken) {
				return model.ErrInvalidToken
			}

			return fmt.Errorf("consume session: %w", err)
		}

		if time.Now().UTC().After(storedToken.ExpiresAt) {
			return model.ErrInvalidToken
		}

		tokens, err = s.generateTokens(ctx, tokenRepoTx, storedToken.UserID)

		return err
	})
	if err != nil {
		if errors.Is(err, model.ErrInvalidToken) {
			log.WarnContext(ctx, "refresh rejected: token is invalid or expired")

			return nil, fmt.Errorf("%s: %w", op, model.ErrInvalidToken)
		}

		log.ErrorContext(ctx, "rotate refresh token", slog.Any("error", err))

		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return tokens, nil
}

func (s *AuthService) Logout(ctx context.Context, userID, refreshToken string) error {
	const op = "authService.Logout"

	log := s.log.With(slog.String("op", op), slog.String("user_id", userID))

	refreshTokenHash := hashToken(refreshToken)

	err := s.tokenRepo.DeleteSessionByHash(ctx, userID, refreshTokenHash)
	if err != nil {
		log.ErrorContext(ctx, "revoke refresh token", slog.Any("error", err))

		return fmt.Errorf("%s: %w", op, err)
	}

	log.InfoContext(ctx, "user logged out")

	return nil
}

func (s *AuthService) ValidateAccessToken(ctx context.Context, token string) (string, error) {
	const (
		op             = "authService.ValidateAccessToken"
		jwtPartsAmount = 3
	)

	log := s.log.With(slog.String("op", op))

	parts := strings.Split(token, ".")
	if len(parts) != jwtPartsAmount {
		log.DebugContext(ctx, "access token is malformed")

		return "", fmt.Errorf("%s: %w", op, model.ErrInvalidAccessToken)
	}

	signingInput := parts[0] + "." + parts[1]

	expectedSignature := signJWT(signingInput, []byte(s.cfg.SETTINGS.JWTSecret))
	if subtle.ConstantTimeCompare([]byte(parts[2]), []byte(expectedSignature)) != 1 {
		log.WarnContext(ctx, "access token signature does not match")

		return "", fmt.Errorf("%s: %w", op, model.ErrInvalidAccessToken)
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		log.DebugContext(ctx, "decode access token payload", slog.Any("error", err))

		return "", fmt.Errorf("%s: %w", op, model.ErrInvalidAccessToken)
	}

	var claims accessTokenClaims

	if err := json.Unmarshal(payload, &claims); err != nil {
		log.DebugContext(ctx, "unmarshal access token claims", slog.Any("error", err))

		return "", fmt.Errorf("%s: %w", op, model.ErrInvalidAccessToken)
	}

	if claims.Sub == "" || claims.Exp <= time.Now().UTC().Unix() {
		log.DebugContext(ctx, "access token is expired or has no subject")

		return "", fmt.Errorf("%s: %w", op, model.ErrInvalidAccessToken)
	}

	return claims.Sub, nil
}

// Membership returns the current role only for an active membership in the requested company.
func (s *AuthService) Membership(ctx context.Context, userID, companyID string) (string, bool, error) {
	const op = "authService.Membership"

	log := s.log.With(slog.String("op", op), slog.String("user_id", userID), slog.String("company_id", companyID))

	var membership model.Membership

	err := s.db.WithContext(ctx).
		Where("user_id = ? AND company_id = ? AND status = ?", userID, companyID, "active").
		Take(&membership).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		log.DebugContext(ctx, "no active membership")

		return "", false, nil
	}

	if err != nil {
		log.ErrorContext(ctx, "select membership", slog.Any("error", err))

		return "", false, fmt.Errorf("%s: %w", op, err)
	}

	return membership.Role, true, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
