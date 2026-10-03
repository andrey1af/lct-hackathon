package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/model"
)

type TokenRepository struct {
	db  gorm.DB
	log *slog.Logger
}

func NewTokenRepo(db gorm.DB, log *slog.Logger) *TokenRepository {
	return &TokenRepository{
		db:  db,
		log: log,
	}
}

func (r *TokenRepository) AddToken(ctx context.Context, refreshToken *model.RefreshToken) error {
	const op = "tokenRepository.AddToken"

	log := r.log.With(slog.String("op", op), slog.String("user_id", refreshToken.UserID))

	err := r.db.WithContext(ctx).Create(refreshToken).Error
	if err != nil {
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
			switch pgErr.Code {
			case "23505":
				log.ErrorContext(ctx, "refresh token hash already exists", slog.Any("error", err))

				return fmt.Errorf("%s: token hash already exists: %w", op, err)

			case "23503":
				log.ErrorContext(ctx, "refresh token owner does not exist", slog.Any("error", err))

				return fmt.Errorf("%s: user does not exist: %w", op, err)
			}
		}

		log.ErrorContext(ctx, "store refresh token", slog.Any("error", err))

		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

// ConsumeSession атомарно находит сессию по хэшу и удаляет её (одноразовое использование refresh-токена).
// Если токена нет в БД, возвращает ErrInvalidToken.
func (r *TokenRepository) ConsumeSession(ctx context.Context, tokenHash string) (*model.RefreshToken, error) {
	const op = "tokenRepository.ConsumeSession"

	log := r.log.With(slog.String("op", op))

	var s model.RefreshToken

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("token_hash = ?", tokenHash).First(&s).Error
		if err != nil {
			return err
		}

		return tx.Delete(&model.RefreshToken{}, s.ID).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		log.WarnContext(ctx, "refresh token is unknown or already used")

		return nil, fmt.Errorf("%s: %w", op, model.ErrInvalidToken)
	}

	if err != nil {
		log.ErrorContext(ctx, "consume refresh token", slog.Any("error", err))

		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &s, nil
}

// DeleteSessionByHash удаляет refresh-токен (logout). Отсутствие записи не считается ошибкой.
func (r *TokenRepository) DeleteSessionByHash(ctx context.Context, userID, tokenHash string) error {
	const op = "tokenRepository.DeleteSessionByHash"

	log := r.log.With(slog.String("op", op), slog.String("user_id", userID))

	err := r.db.WithContext(ctx).Where("user_id = ? AND token_hash = ?", userID, tokenHash).Delete(&model.RefreshToken{}).Error
	if err != nil {
		log.ErrorContext(ctx, "delete refresh token", slog.Any("error", err))

		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

// DeleteExpiredSessions чистит просроченные refresh-токены пользователя.
func (r *TokenRepository) DeleteExpiredSessions(ctx context.Context, userID string) error {
	const op = "tokenRepository.DeleteExpiredSessions"

	log := r.log.With(slog.String("op", op), slog.String("user_id", userID))

	err := r.db.WithContext(ctx).
		Where("user_id = ? AND expires_at < ?", userID, time.Now()).
		Delete(&model.RefreshToken{}).Error
	if err != nil {
		log.ErrorContext(ctx, "delete expired refresh tokens", slog.Any("error", err))

		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (r *TokenRepository) WithTx(tx *gorm.DB) *TokenRepository {
	return &TokenRepository{db: *tx, log: r.log}
}
