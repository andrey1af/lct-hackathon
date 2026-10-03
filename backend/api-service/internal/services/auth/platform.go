package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"uuid"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/model"
)

var (
	ErrForbidden = errors.New("company admin required")
	ErrNotFound  = errors.New("resource not found")
	ErrConflict  = errors.New("resource conflicts with current state")
)

type CompanyView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

type Profile struct {
	ID        string        `json:"id"`
	Email     string        `json:"email"`
	Name      string        `json:"name"`
	Companies []CompanyView `json:"companies"`
}

type MemberView struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	Status string `json:"status"`
}

type InvitationView struct {
	Token     string    `json:"token"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	ExpiresAt time.Time `json:"expires_at"`
}

type SessionHistoryView struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Mode      string    `json:"mode"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *AuthService) Profile(ctx context.Context, userID string) (Profile, error) {
	const op = "authService.Profile"

	log := s.log.With(slog.String("op", op), slog.String("user_id", userID))

	user, err := s.userRepo.GetUserByID(ctx, userID)
	if errors.Is(err, model.ErrUserNotFound) {
		log.WarnContext(ctx, "profile requested for unknown user")

		return Profile{}, fmt.Errorf("%s: %w", op, ErrNotFound)
	}

	if err != nil {
		log.ErrorContext(ctx, "get user by id", slog.Any("error", err))

		return Profile{}, fmt.Errorf("%s: %w", op, err)
	}

	var companies []CompanyView

	err = s.db.WithContext(ctx).Table("memberships").
		Select("companies.id, companies.name, memberships.role").
		Joins("JOIN companies ON companies.id = memberships.company_id").
		Where("memberships.user_id = ? AND memberships.status = ?", userID, "active").
		Order("companies.name").Scan(&companies).Error
	if err != nil {
		log.ErrorContext(ctx, "select user companies", slog.Any("error", err))

		return Profile{}, fmt.Errorf("%s: %w", op, err)
	}

	return Profile{ID: user.ID, Email: user.Email, Name: user.Name, Companies: companies}, nil
}

func (s *AuthService) UpdateProfile(ctx context.Context, userID, name string) (Profile, error) {
	const op = "authService.UpdateProfile"

	log := s.log.With(slog.String("op", op), slog.String("user_id", userID))

	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 {
		log.WarnContext(ctx, "profile update rejected: invalid name")

		return Profile{}, fmt.Errorf("%s: %w", op, model.ErrInvalidInput)
	}

	update := s.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", userID).Update("name", name)
	if update.Error != nil {
		log.ErrorContext(ctx, "update user name", slog.Any("error", update.Error))

		return Profile{}, fmt.Errorf("%s: %w", op, update.Error)
	}

	if update.RowsAffected == 0 {
		log.WarnContext(ctx, "profile update affected no rows")

		return Profile{}, fmt.Errorf("%s: %w", op, ErrNotFound)
	}

	return s.Profile(ctx, userID)
}

func (s *AuthService) requireAdmin(ctx context.Context, tx *gorm.DB, companyID, userID string) error {
	const op = "authService.requireAdmin"

	log := s.log.With(slog.String("op", op), slog.String("company_id", companyID), slog.String("user_id", userID))

	var membership model.Membership

	err := tx.Where("company_id = ? AND user_id = ? AND status = ?", companyID, userID, "active").Take(&membership).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		log.WarnContext(ctx, "no active membership in company")

		return fmt.Errorf("%s: %w", op, ErrNotFound)
	}

	if err != nil {
		log.ErrorContext(ctx, "select membership", slog.Any("error", err))

		return fmt.Errorf("%s: %w", op, err)
	}

	if membership.Role != "company_admin" && membership.Role != "platform_admin" {
		log.WarnContext(ctx, "company admin rights required", slog.String("role", membership.Role))

		return fmt.Errorf("%s: %w", op, ErrForbidden)
	}

	return nil
}

func (s *AuthService) Members(ctx context.Context, companyID, adminID string) ([]MemberView, error) {
	const op = "authService.Members"

	log := s.log.With(slog.String("op", op), slog.String("company_id", companyID), slog.String("user_id", adminID))

	db := s.db.WithContext(ctx)
	if err := s.requireAdmin(ctx, db, companyID, adminID); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	var members []MemberView

	err := db.Table("memberships").Select("users.id AS user_id, users.email, users.name, memberships.role, memberships.status").
		Joins("JOIN users ON users.id = memberships.user_id").Where("memberships.company_id = ?", companyID).
		Order("users.name, users.id").Scan(&members).Error
	if err != nil {
		log.ErrorContext(ctx, "select company members", slog.Any("error", err))

		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return members, nil
}

func (s *AuthService) UpdateMember(ctx context.Context, companyID, adminID, targetID, role, status string) error {
	const op = "authService.UpdateMember"

	log := s.log.With(
		slog.String("op", op),
		slog.String("company_id", companyID),
		slog.String("user_id", adminID),
		slog.String("target_user_id", targetID),
	)

	if role != "employee" && role != "company_admin" {
		log.WarnContext(ctx, "member update rejected: unknown role", slog.String("role", role))

		return fmt.Errorf("%s: %w", op, model.ErrInvalidInput)
	}

	if status != "active" && status != "inactive" {
		log.WarnContext(ctx, "member update rejected: unknown status", slog.String("status", status))

		return fmt.Errorf("%s: %w", op, model.ErrInvalidInput)
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", companyID).Error; err != nil {
			return err
		}

		if err := s.requireAdmin(ctx, tx, companyID, adminID); err != nil {
			return err
		}

		var current model.Membership

		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("company_id = ? AND user_id = ?", companyID, targetID).Take(&current).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}

		if err != nil {
			return err
		}

		if current.Role == "company_admin" && current.Status == "active" && (role != "company_admin" || status != "active") {
			var count int64

			if err := tx.Model(&model.Membership{}).Where("company_id = ? AND role = ? AND status = ?", companyID, "company_admin", "active").Count(&count).Error; err != nil {
				return err
			}

			if count <= 1 {
				return ErrConflict
			}
		}

		return tx.Model(&current).Updates(map[string]any{"role": role, "status": status}).Error
	})
	if err != nil {
		if errors.Is(err, ErrForbidden) || errors.Is(err, ErrNotFound) || errors.Is(err, ErrConflict) {
			log.WarnContext(ctx, "member update rejected", slog.Any("error", err))
		} else {
			log.ErrorContext(ctx, "update member", slog.Any("error", err))
		}

		return fmt.Errorf("%s: %w", op, err)
	}

	log.InfoContext(ctx, "member updated", slog.String("role", role), slog.String("status", status))

	return nil
}

func (s *AuthService) CreateInvitation(ctx context.Context, companyID, adminID, email, role string) (InvitationView, error) {
	const op = "authService.CreateInvitation"

	log := s.log.With(slog.String("op", op), slog.String("company_id", companyID), slog.String("user_id", adminID))

	email = normalizeEmail(email)
	if email == "" || (role != "employee" && role != "company_admin") {
		log.WarnContext(ctx, "invitation rejected: empty email or unknown role", slog.String("role", role))

		return InvitationView{}, fmt.Errorf("%s: %w", op, model.ErrInvalidInput)
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		log.ErrorContext(ctx, "read random bytes", slog.Any("error", err))

		return InvitationView{}, fmt.Errorf("%s: generate invitation token: %w", op, err)
	}

	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	expires := time.Now().UTC().Add(7 * 24 * time.Hour)

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireAdmin(ctx, tx, companyID, adminID); err != nil {
			return err
		}

		invitation := model.Invitation{ID: uuid.New().String(), CompanyID: companyID, Email: email, Role: role, TokenHash: hashToken(token), ExpiresAt: expires, CreatedAt: time.Now().UTC()}

		return tx.Create(&invitation).Error
	})
	if err != nil {
		if errors.Is(err, ErrForbidden) || errors.Is(err, ErrNotFound) {
			log.WarnContext(ctx, "invitation rejected", slog.Any("error", err))
		} else {
			log.ErrorContext(ctx, "store invitation", slog.Any("error", err))
		}

		return InvitationView{}, fmt.Errorf("%s: %w", op, err)
	}

	log.InfoContext(ctx, "invitation created", slog.String("role", role), slog.Time("expires_at", expires))

	return InvitationView{Token: token, Email: email, Role: role, ExpiresAt: expires}, nil
}

func (s *AuthService) AcceptInvitation(ctx context.Context, token, loggedUserID, name, password string) (Profile, error) {
	const op = "authService.AcceptInvitation"

	log := s.log.With(slog.String("op", op))

	if token == "" {
		log.WarnContext(ctx, "invitation accept rejected: empty token")

		return Profile{}, fmt.Errorf("%s: %w", op, model.ErrInvalidInput)
	}

	var userID string

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var invitation model.Invitation

		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("token_hash = ?", hashToken(token)).Take(&invitation).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}

		if err != nil {
			return err
		}

		if invitation.AcceptedAt != nil || !time.Now().UTC().Before(invitation.ExpiresAt) {
			return ErrConflict
		}

		var user model.User

		err = tx.Where("email = ?", invitation.Email).Take(&user).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			name = strings.TrimSpace(name)
			if loggedUserID != "" || name == "" || len(password) < minPasswordLen || len(password) > maxPasswordLen {
				return model.ErrInvalidInput
			}

			hash, hashErr := hashPassword(password)
			if hashErr != nil {
				return hashErr
			}

			user = model.User{ID: uuid.New().String(), Email: invitation.Email, Name: name, PasswordHash: string(hash)}
			if err := tx.Create(&user).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if loggedUserID != user.ID {
			return model.ErrInvalidCredentials
		}

		userID = user.ID
		membership := model.Membership{CompanyID: invitation.CompanyID, UserID: user.ID, Role: invitation.Role, Status: "active"}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "company_id"}, {Name: "user_id"}}, DoUpdates: clause.AssignmentColumns([]string{"role", "status"})}).Create(&membership).Error; err != nil {
			return err
		}

		now := time.Now().UTC()

		return tx.Model(&invitation).Update("accepted_at", now).Error
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrConflict) || errors.Is(err, model.ErrInvalidInput) || errors.Is(err, model.ErrInvalidCredentials) {
			log.WarnContext(ctx, "invitation accept rejected", slog.Any("error", err))
		} else {
			log.ErrorContext(ctx, "accept invitation", slog.Any("error", err))
		}

		return Profile{}, fmt.Errorf("%s: %w", op, err)
	}

	log.InfoContext(ctx, "invitation accepted", slog.String("user_id", userID))

	return s.Profile(ctx, userID)
}

func (s *AuthService) Sessions(ctx context.Context, companyID, userID string) ([]SessionHistoryView, error) {
	const op = "authService.Sessions"

	log := s.log.With(slog.String("op", op), slog.String("company_id", companyID), slog.String("user_id", userID))

	var sessions []SessionHistoryView

	err := s.db.WithContext(ctx).Table("sessions").Select("sessions.id, case_versions.title, sessions.mode, sessions.status, sessions.created_at").
		Joins("JOIN case_versions ON case_versions.id = sessions.case_version_id").
		Where("sessions.company_id = ? AND sessions.user_id = ?", companyID, userID).
		Order("sessions.created_at DESC").Scan(&sessions).Error
	if err != nil {
		log.ErrorContext(ctx, "select session history", slog.Any("error", err))

		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return sessions, nil
}

func (s *AuthService) BootstrapAdmin(ctx context.Context, companyName, email, name, password string) (string, error) {
	const op = "authService.BootstrapAdmin"

	log := s.log.With(slog.String("op", op))

	companyName, email, name = strings.TrimSpace(companyName), normalizeEmail(email), strings.TrimSpace(name)
	if companyName == "" || email == "" || name == "" || len(password) < minPasswordLen || len(password) > maxPasswordLen {
		log.WarnContext(ctx, "bootstrap rejected: invalid company, email, name or password")

		return "", fmt.Errorf("%s: %w", op, model.ErrInvalidInput)
	}

	hash, err := hashPassword(password)
	if err != nil {
		log.ErrorContext(ctx, "hash password", slog.Any("error", err))

		return "", fmt.Errorf("%s: %w", op, err)
	}

	companyID := uuid.New().String()
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", companyName).Error; err != nil {
			return err
		}

		var duplicates int64

		if err := tx.Model(&model.Company{}).Where("name = ?", companyName).Count(&duplicates).Error; err != nil {
			return err
		}

		if duplicates != 0 {
			return ErrConflict
		}

		var user model.User

		err := tx.Where("email = ?", email).Take(&user).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			user = model.User{ID: uuid.New().String(), Email: email, Name: name, PasswordHash: string(hash)}
			if err := tx.Create(&user).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			log.WarnContext(ctx, "bootstrap reuses an existing user, password and name left unchanged", slog.String("user_id", user.ID))
		}

		if err := tx.Create(&model.Company{ID: companyID, Name: companyName}).Error; err != nil {
			return err
		}

		membership := model.Membership{CompanyID: companyID, UserID: user.ID, Role: "company_admin", Status: "active"}

		return tx.Create(&membership).Error
	})
	if err != nil {
		if errors.Is(err, ErrConflict) {
			log.WarnContext(ctx, "bootstrap rejected: company with the same name already exists", slog.String("company_name", companyName))
		} else {
			log.ErrorContext(ctx, "bootstrap company", slog.Any("error", err))
		}

		return "", fmt.Errorf("%s: %w", op, err)
	}

	log.InfoContext(ctx, "company and admin created", slog.String("company_id", companyID))

	return companyID, nil
}
