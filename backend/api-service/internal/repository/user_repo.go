package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"gorm.io/gorm"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/model"
)

type UserRepository struct {
	db  gorm.DB
	log *slog.Logger
}

func NewUserRepo(db gorm.DB, log *slog.Logger) *UserRepository {
	return &UserRepository{
		db:  db,
		log: log,
	}
}

func (r *UserRepository) CreateUser(ctx context.Context, u *model.User) error {
	const op = "userRepository.CreateUser"

	log := r.log.With(slog.String("op", op))

	err := r.db.WithContext(ctx).Create(u).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		log.WarnContext(ctx, "user email is already taken", slog.String("user_id", u.ID))

		return fmt.Errorf("%s: %w", op, model.ErrEmailTaken)
	}

	if err != nil {
		log.ErrorContext(ctx, "create user", slog.String("user_id", u.ID), slog.Any("error", err))

		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (r *UserRepository) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	const op = "userRepository.GetUserByEmail"

	log := r.log.With(slog.String("op", op))

	var u model.User

	err := r.db.WithContext(ctx).Where("email = ?", email).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%s: %w", op, model.ErrUserNotFound)
	}

	if err != nil {
		log.ErrorContext(ctx, "select user by email", slog.Any("error", err))

		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &u, nil
}

func (r *UserRepository) GetUserByID(ctx context.Context, id string) (*model.User, error) {
	const op = "userRepository.GetUserByID"

	log := r.log.With(slog.String("op", op))

	var u model.User

	err := r.db.WithContext(ctx).Where("id = ?", id).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%s: %w", op, model.ErrUserNotFound)
	}

	if err != nil {
		log.ErrorContext(ctx, "select user by id", slog.String("user_id", id), slog.Any("error", err))

		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &u, nil
}

func (r *UserRepository) WithTx(tx *gorm.DB) UserRepository {
	return UserRepository{db: *tx, log: r.log}
}
