package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/model"
)

const refreshTokenSize = 32

type tokenAdder interface {
	AddToken(ctx context.Context, refreshToken *model.RefreshToken) error
}

type accessTokenClaims struct {
	Sub string `json:"sub"`
	Exp int64  `json:"exp"`
	Iat int64  `json:"iat"`
}

func (s *AuthService) generateTokens(ctx context.Context, tokenRepo tokenAdder, userID string) (*model.JWT, error) {
	const op = "authService.generateTokens"

	log := s.log.With(slog.String("op", op), slog.String("user_id", userID))

	accessToken, err := s.newAccessToken(ctx, userID)
	if err != nil {
		log.ErrorContext(ctx, "build access token", slog.Any("error", err))

		return nil, fmt.Errorf("%s: %w", op, err)
	}

	refreshToken, refreshTokenHash, refreshExpiresAt, err := s.newRefreshToken(ctx)
	if err != nil {
		log.ErrorContext(ctx, "build refresh token", slog.Any("error", err))

		return nil, fmt.Errorf("%s: %w", op, err)
	}

	if err := tokenRepo.AddToken(ctx, &model.RefreshToken{
		UserID:    userID,
		TokenHash: refreshTokenHash,
		ExpiresAt: refreshExpiresAt,
		CreatedAt: time.Now(),
	}); err != nil {
		log.ErrorContext(ctx, "store refresh token", slog.Any("error", err))

		return nil, fmt.Errorf("%s: store refresh token: %w", op, err)
	}

	log.DebugContext(ctx, "token pair issued", slog.Time("refresh_expires_at", refreshExpiresAt))

	return &model.JWT{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (s *AuthService) newAccessToken(ctx context.Context, userID string) (string, error) {
	const op = "authService.newAccessToken"

	log := s.log.With(slog.String("op", op), slog.String("user_id", userID))

	now := time.Now().UTC()

	header, err := json.Marshal(map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	})
	if err != nil {
		log.ErrorContext(ctx, "marshal jwt header", slog.Any("error", err))

		return "", fmt.Errorf("%s: marshal jwt header: %w", op, err)
	}

	payload, err := json.Marshal(accessTokenClaims{
		Sub: userID,
		Exp: now.Add(s.cfg.SETTINGS.AccessTokenTTL).Unix(),
		Iat: now.Unix(),
	})
	if err != nil {
		log.ErrorContext(ctx, "marshal jwt payload", slog.Any("error", err))

		return "", fmt.Errorf("%s: marshal jwt payload: %w", op, err)
	}

	signingInput := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	signature := signJWT(signingInput, []byte(s.cfg.SETTINGS.JWTSecret))

	return signingInput + "." + signature, nil
}

func (s *AuthService) newRefreshToken(ctx context.Context) (
	token string,
	tokenHash string,
	expiresAt time.Time,
	err error,
) {
	const op = "authService.newRefreshToken"

	log := s.log.With(slog.String("op", op))

	buf := make([]byte, refreshTokenSize)
	if _, err = rand.Read(buf); err != nil {
		log.ErrorContext(ctx, "read random bytes", slog.Any("error", err))

		return "", "", time.Time{}, fmt.Errorf("%s: generate refresh token: %w", op, err)
	}

	token = base64.RawURLEncoding.EncodeToString(buf)
	tokenHash = hashToken(token)
	expiresAt = time.Now().UTC().Add(s.cfg.SETTINGS.RefreshTokenTTL)

	return token, tokenHash, expiresAt, nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))

	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func signJWT(signingInput string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(signingInput))

	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
