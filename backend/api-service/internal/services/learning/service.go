package learning

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"
	"uuid"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/content"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/model"
)

var (
	ErrNotFound = errors.New("module or step not found")
	ErrConflict = errors.New("step is closed or attempt key conflicts")
	ErrInvalid  = errors.New("invalid answer")
)

type ModuleView struct {
	ID             string   `json:"id"`
	Version        int      `json:"version"`
	Title          string   `json:"title"`
	Order          int      `json:"order"`
	SkillIDs       []string `json:"skill_ids"`
	Status         string   `json:"status"`
	CompletedSteps int      `json:"completed_steps"`
	TotalSteps     int      `json:"total_steps"`
}

type StepView struct {
	ID        string           `json:"id"`
	Order     int              `json:"order"`
	Type      string           `json:"type"`
	Content   string           `json:"content"`
	Options   []content.Option `json:"options"`
	Completed bool             `json:"completed"`
}

type ModuleDetail struct {
	ModuleView
	Steps []StepView `json:"steps"`
}

type TeamMemberProgress struct {
	UserID  string       `json:"user_id"`
	Name    string       `json:"name"`
	Email   string       `json:"email"`
	Role    string       `json:"role"`
	Modules []ModuleView `json:"modules"`
}

type AttemptResult struct {
	Correct        bool   `json:"correct"`
	Completed      bool   `json:"completed"`
	Explanation    string `json:"explanation"`
	ModuleStatus   string `json:"module_status"`
	CompletedSteps int    `json:"completed_steps"`
	TotalSteps     int    `json:"total_steps"`
}

type Service struct {
	db  *gorm.DB
	log *slog.Logger
}

func NewService(db *gorm.DB, log *slog.Logger) *Service {
	return &Service{
		db:  db,
		log: log,
	}
}

func (s *Service) TeamProgress(ctx context.Context, companyID string) ([]TeamMemberProgress, error) {
	const op = "learningService.TeamProgress"

	log := s.log.With(slog.String("op", op), slog.String("company_id", companyID))

	var rows []struct {
		UserID string
		Name   string
		Email  string
		Role   string
	}

	err := s.db.WithContext(ctx).Table("memberships").
		Select("memberships.user_id, users.name, users.email, memberships.role").
		Joins("JOIN users ON users.id = memberships.user_id").
		Where("memberships.company_id = ? AND memberships.status = ?", companyID, "active").
		Order("users.name, users.id").Scan(&rows).Error
	if err != nil {
		log.ErrorContext(ctx, "select company members", slog.Any("error", err))

		return nil, fmt.Errorf("%s: %w", op, err)
	}

	result := make([]TeamMemberProgress, 0, len(rows))

	for _, row := range rows {
		modules, err := s.ListModules(ctx, companyID, row.UserID)
		if err != nil {
			log.ErrorContext(ctx, "list modules for member", slog.String("user_id", row.UserID), slog.Any("error", err))

			return nil, fmt.Errorf("%s: %w", op, err)
		}

		result = append(result, TeamMemberProgress{UserID: row.UserID, Name: row.Name, Email: row.Email, Role: row.Role, Modules: modules})
	}

	return result, nil
}

func (s *Service) ListModules(ctx context.Context, companyID, userID string) ([]ModuleView, error) {
	const op = "learningService.ListModules"

	log := s.log.With(slog.String("op", op), slog.String("company_id", companyID), slog.String("user_id", userID))

	versions, err := currentVersions(s.db.WithContext(ctx))
	if err != nil {
		log.ErrorContext(ctx, "select current module versions", slog.Any("error", err))

		return nil, fmt.Errorf("%s: %w", op, err)
	}

	views := make([]ModuleView, 0, len(versions))
	previousPassed := true

	for _, version := range versions {
		view, err := s.moduleView(ctx, s.db.WithContext(ctx), version, companyID, userID, previousPassed)
		if err != nil {
			log.ErrorContext(ctx, "build module view", slog.String("module_version_id", version.ID), slog.Any("error", err))

			return nil, fmt.Errorf("%s: %w", op, err)
		}

		views = append(views, view)
		previousPassed = view.Status == "passed"
	}

	return views, nil
}

func (s *Service) GetModule(ctx context.Context, companyID, userID, moduleID string) (ModuleDetail, error) {
	const op = "learningService.GetModule"

	log := s.log.With(
		slog.String("op", op),
		slog.String("company_id", companyID),
		slog.String("user_id", userID),
		slog.String("module_version_id", moduleID),
	)

	views, err := s.ListModules(ctx, companyID, userID)
	if err != nil {
		return ModuleDetail{}, fmt.Errorf("%s: %w", op, err)
	}

	var view *ModuleView

	for i := range views {
		if views[i].ID == moduleID {
			view = &views[i]
			break
		}
	}

	if view == nil || view.Status == "locked" {
		log.WarnContext(ctx, "module is unknown or locked")

		return ModuleDetail{}, fmt.Errorf("%s: %w", op, ErrNotFound)
	}

	var steps []model.Step

	if err := s.db.WithContext(ctx).Where("module_version_id = ?", moduleID).Order("order_index").Find(&steps).Error; err != nil {
		log.ErrorContext(ctx, "select module steps", slog.Any("error", err))

		return ModuleDetail{}, fmt.Errorf("%s: %w", op, err)
	}

	completed, err := completedStepIDs(s.db.WithContext(ctx), companyID, userID, moduleID)
	if err != nil {
		log.ErrorContext(ctx, "select completed steps", slog.Any("error", err))

		return ModuleDetail{}, fmt.Errorf("%s: %w", op, err)
	}

	result := ModuleDetail{ModuleView: *view, Steps: make([]StepView, 0, len(steps))}

	for _, step := range steps {
		var options []content.Option

		if err := json.Unmarshal(step.Options, &options); err != nil {
			log.ErrorContext(ctx, "decode step options", slog.String("step_id", step.ID), slog.Any("error", err))

			return ModuleDetail{}, fmt.Errorf("%s: decode step options: %w", op, err)
		}

		result.Steps = append(result.Steps, StepView{ID: step.ID, Order: step.OrderIndex, Type: step.Type, Content: step.Content, Options: options, Completed: completed[step.ID]})
	}

	return result, nil
}

func (s *Service) SubmitAttempt(ctx context.Context, companyID, userID, stepID, clientAttemptID, answer string) (AttemptResult, error) {
	const op = "learningService.SubmitAttempt"

	log := s.log.With(
		slog.String("op", op),
		slog.String("company_id", companyID),
		slog.String("user_id", userID),
		slog.String("step_id", stepID),
	)

	if _, err := uuid.Parse(stepID); err != nil {
		log.WarnContext(ctx, "attempt rejected: step id is not a uuid")

		return AttemptResult{}, fmt.Errorf("%s: %w", op, ErrNotFound)
	}

	if clientAttemptID == "" || len(clientAttemptID) > 128 {
		log.WarnContext(ctx, "attempt rejected: invalid client attempt id")

		return AttemptResult{}, fmt.Errorf("%s: %w", op, ErrInvalid)
	}

	var result AttemptResult

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtext(?), hashtext(?))`, companyID, userID).Error; err != nil {
			return err
		}

		var prior model.LearningAttempt

		err := tx.Where("company_id = ? AND user_id = ? AND client_attempt_id = ?", companyID, userID, clientAttemptID).Take(&prior).Error
		if err == nil {
			if prior.StepID != stepID || prior.Answer != answer {
				return ErrConflict
			}

			return json.Unmarshal(prior.ResultJSON, &result)
		}

		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var step model.Step

		if err := tx.Where("id = ?", stepID).Take(&step).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}

			return err
		}

		versions, err := currentVersions(tx)
		if err != nil {
			return err
		}

		index := slices.IndexFunc(versions, func(v model.ModuleVersion) bool { return v.ID == step.ModuleVersionID })
		if index < 0 {
			return ErrNotFound
		}

		if index > 0 {
			previous, err := s.moduleView(ctx, tx, versions[index-1], companyID, userID, true)
			if err != nil {
				return err
			}

			if previous.Status != "passed" {
				return ErrNotFound
			}
		}

		var progress model.ModuleProgress

		err = tx.Where("company_id = ? AND user_id = ? AND module_version_id = ?", companyID, userID, step.ModuleVersionID).Take(&progress).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if progress.Status == "passed" {
			return ErrConflict
		}

		completed, err := completedStepIDs(tx, companyID, userID, step.ModuleVersionID)
		if err != nil {
			return err
		}

		if completed[step.ID] {
			return ErrConflict
		}

		var steps []model.Step

		if err := tx.Where("module_version_id = ?", step.ModuleVersionID).Order("order_index").Find(&steps).Error; err != nil {
			return err
		}

		for _, earlier := range steps {
			if earlier.OrderIndex >= step.OrderIndex {
				break
			}

			if !completed[earlier.ID] {
				return ErrConflict
			}
		}

		if step.Type == "theory" {
			if answer != "confirmed" {
				return ErrInvalid
			}
		} else {
			var options []content.Option

			if err := json.Unmarshal(step.Options, &options); err != nil {
				return err
			}

			if !slices.ContainsFunc(options, func(o content.Option) bool { return o.ID == answer }) {
				return ErrInvalid
			}
		}

		correct := answer == step.AnswerKey
		completedCount := len(completed)
		if correct {
			completedCount++
		}

		status := "in_progress"
		if completedCount == len(steps) {
			status = "assessment_ready"
		} else if progress.Status == "needs_retry" {
			status = "needs_retry"
		}

		result = AttemptResult{Correct: correct, Completed: correct, Explanation: step.Explanation, ModuleStatus: status, CompletedSteps: completedCount, TotalSteps: len(steps)}
		resultJSON, err := json.Marshal(result)
		if err != nil {
			return err
		}

		attempt := model.LearningAttempt{ID: uuid.New().String(), CompanyID: companyID, UserID: userID, StepID: stepID, ModuleVersionID: step.ModuleVersionID, ClientAttemptID: clientAttemptID, Answer: answer, Correct: correct, Completed: correct, ResultJSON: resultJSON, CreatedAt: time.Now().UTC()}
		if err := tx.Create(&attempt).Error; err != nil {
			return err
		}

		progress = model.ModuleProgress{CompanyID: companyID, UserID: userID, ModuleVersionID: step.ModuleVersionID, Status: status, UpdatedAt: time.Now().UTC()}

		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "company_id"}, {Name: "user_id"}, {Name: "module_version_id"}}, DoUpdates: clause.AssignmentColumns([]string{"status", "updated_at"})}).Create(&progress).Error
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrConflict) || errors.Is(err, ErrInvalid) {
			log.WarnContext(ctx, "attempt rejected", slog.Any("error", err))
		} else {
			log.ErrorContext(ctx, "store learning attempt", slog.Any("error", err))
		}

		return AttemptResult{}, fmt.Errorf("%s: %w", op, err)
	}

	log.InfoContext(ctx, "attempt accepted",
		slog.Bool("correct", result.Correct),
		slog.String("module_status", result.ModuleStatus),
		slog.Int("completed_steps", result.CompletedSteps),
	)

	return result, nil
}

func (s *Service) moduleView(ctx context.Context, tx *gorm.DB, version model.ModuleVersion, companyID, userID string, previousPassed bool) (ModuleView, error) {
	const op = "learningService.moduleView"

	log := s.log.With(slog.String("op", op), slog.String("module_version_id", version.ID), slog.String("user_id", userID))

	var count int64

	if err := tx.Model(&model.Step{}).Where("module_version_id = ?", version.ID).Count(&count).Error; err != nil {
		log.ErrorContext(ctx, "count module steps", slog.Any("error", err))

		return ModuleView{}, fmt.Errorf("%s: %w", op, err)
	}

	completed, err := completedStepIDs(tx, companyID, userID, version.ID)
	if err != nil {
		log.ErrorContext(ctx, "select completed steps", slog.Any("error", err))

		return ModuleView{}, fmt.Errorf("%s: %w", op, err)
	}

	var progress model.ModuleProgress

	err = tx.Where("company_id = ? AND user_id = ? AND module_version_id = ?", companyID, userID, version.ID).Take(&progress).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.ErrorContext(ctx, "select module progress", slog.Any("error", err))

		return ModuleView{}, fmt.Errorf("%s: %w", op, err)
	}

	status := "locked"
	if previousPassed {
		switch {
		case progress.Status == "passed" || progress.Status == "needs_retry":
			status = progress.Status

		case int64(len(completed)) == count && count > 0:
			status = "assessment_ready"

		case len(completed) > 0 || progress.Status == "in_progress":
			status = "in_progress"

		default:
			status = "available"
		}
	}

	var skills []string

	if err := json.Unmarshal(version.SkillIDs, &skills); err != nil {
		log.ErrorContext(ctx, "decode module skills", slog.Any("error", err))

		return ModuleView{}, fmt.Errorf("%s: decode module skills: %w", op, err)
	}

	return ModuleView{ID: version.ID, Version: version.Version, Title: version.Title, Order: version.OrderIndex, SkillIDs: skills, Status: status, CompletedSteps: len(completed), TotalSteps: int(count)}, nil
}

func currentVersions(tx *gorm.DB) ([]model.ModuleVersion, error) {
	var versions []model.ModuleVersion

	err := tx.Where("(module_id, version) IN (SELECT module_id, MAX(version) FROM module_versions GROUP BY module_id)").Order("order_index").Find(&versions).Error

	return versions, err
}

func completedStepIDs(tx *gorm.DB, companyID, userID, moduleVersionID string) (map[string]bool, error) {
	var ids []string

	err := tx.Model(&model.LearningAttempt{}).Distinct("step_id").Where("company_id = ? AND user_id = ? AND module_version_id = ? AND completed = true", companyID, userID, moduleVersionID).Pluck("step_id", &ids).Error
	if err != nil {
		return nil, err
	}

	result := make(map[string]bool, len(ids))

	for _, id := range ids {
		result[id] = true
	}

	return result, nil
}
