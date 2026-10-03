package negotiation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
	"uuid"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/llm"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/model"
)

var (
	ErrNotFound  = errors.New("session or scenario not found")
	ErrConflict  = errors.New("session is not active")
	ErrInvalid   = errors.New("invalid request")
	ErrLocked    = errors.New("assessment is not unlocked yet")
	ErrForbidden = errors.New("action is not available for this case")
)

const historyLimit = 12

const (
	defaultHintLimit         = 3
	defaultReplyTimeout      = 45 * time.Second
	defaultAssessmentTimeout = 90 * time.Second
)

const (
	modePractice   = "practice"
	modeAssessment = "assessment"
)

const (
	roleUser     = "user"
	roleOpponent = "opponent"
)

const (
	statusActive    = "active"
	statusFinished  = "finished"
	statusEvaluated = "evaluated"
)

const (
	progressPassed     = "passed"
	progressNeedsRetry = "needs_retry"
	progressReady      = "assessment_ready"
)

var assessmentUnlockedBy = []string{progressReady, progressNeedsRetry, progressPassed}

type Config struct {
	DefaultCaseKey    string
	HintLimit         int
	ReplyTimeout      time.Duration
	AssessmentTimeout time.Duration
}

func (c Config) withDefaults() Config {
	if c.HintLimit <= 0 {
		c.HintLimit = defaultHintLimit
	}

	if c.ReplyTimeout <= 0 {
		c.ReplyTimeout = defaultReplyTimeout
	}

	if c.AssessmentTimeout <= 0 {
		c.AssessmentTimeout = defaultAssessmentTimeout
	}

	return c
}

type CaseView struct {
	Key            string   `json:"key"`
	Title          string   `json:"title"`
	Mode           string   `json:"mode"`
	Difficulty     string   `json:"difficulty"`
	SkillIDs       []string `json:"skill_ids"`
	MaxTurns       int      `json:"max_turns"`
	Briefing       string   `json:"briefing"`
	HintsAvailable bool     `json:"hints_available"`
	Locked         bool     `json:"locked"`
	UnlockedBy     string   `json:"unlocked_by,omitempty"`
}

type MessageView struct {
	Role       string    `json:"role"`
	TurnNumber int       `json:"turn_number"`
	Text       string    `json:"text"`
	CreatedAt  time.Time `json:"created_at"`
}

type SessionView struct {
	SessionID string        `json:"session_id"`
	Status    string        `json:"status"`
	Mode      string        `json:"mode"`
	CaseKey   string        `json:"case_key"`
	Title     string        `json:"title"`
	Briefing  string        `json:"briefing"`
	Attempt   int           `json:"attempt"`
	TurnsUsed int           `json:"turns_used"`
	MaxTurns  int           `json:"max_turns"`
	HintsUsed int           `json:"hints_used"`
	HintsLeft int           `json:"hints_left"`
	Messages  []MessageView `json:"messages"`
}

type TurnView struct {
	SessionID     string      `json:"session_id"`
	Status        string      `json:"status"`
	TurnNumber    int         `json:"turn_number"`
	PlayerMessage MessageView `json:"player_message"`
	Opponent      MessageView `json:"opponent_message"`
	TurnsUsed     int         `json:"turns_used"`
	MaxTurns      int         `json:"max_turns"`
}

type HintView struct {
	SessionID string `json:"session_id"`
	Hint      string `json:"hint"`
	HintsUsed int    `json:"hints_used"`
	HintsLeft int    `json:"hints_left"`
}

type sessionState struct {
	HintsUsed int `json:"hints_used"`
}

type Service struct {
	db       *gorm.DB
	provider llm.Provider
	cfg      Config
	log      *slog.Logger
}

func NewService(db *gorm.DB, provider llm.Provider, cfg Config, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}

	return &Service{db: db, provider: provider, cfg: cfg.withDefaults(), log: logger}
}

func (s *Service) ListCases(ctx context.Context, companyID, userID string) ([]CaseView, error) {
	const op = "negotiationService.ListCases"

	log := s.log.With(slog.String("op", op), slog.String("company_id", companyID), slog.String("user_id", userID))

	db := s.db.WithContext(ctx)

	var scenarios []model.Case

	if err := db.Where("company_id IS NULL OR company_id = ?", companyID).Find(&scenarios).Error; err != nil {
		log.ErrorContext(ctx, "select cases", slog.Any("error", err))

		return nil, fmt.Errorf("%s: %w", op, err)
	}

	if len(scenarios) == 0 {
		return []CaseView{}, nil
	}

	keys := make(map[string]string, len(scenarios))

	for _, scenario := range scenarios {
		keys[scenario.ID] = scenario.Key
	}

	var versions []model.CaseVersion

	if err := db.Order("case_id, version").Find(&versions).Error; err != nil {
		log.ErrorContext(ctx, "select case versions", slog.Any("error", err))

		return nil, fmt.Errorf("%s: %w", op, err)
	}

	latest := make(map[string]model.CaseVersion, len(versions))

	for _, version := range versions {
		if _, visible := keys[version.CaseID]; visible {
			latest[version.CaseID] = version
		}
	}

	modules, err := assessmentModules(db)
	if err != nil {
		log.ErrorContext(ctx, "select assessment modules", slog.Any("error", err))

		return nil, fmt.Errorf("%s: %w", op, err)
	}

	statuses, err := progressStatuses(db, companyID, userID)
	if err != nil {
		log.ErrorContext(ctx, "select module progress", slog.Any("error", err))

		return nil, fmt.Errorf("%s: %w", op, err)
	}

	views := make([]CaseView, 0, len(latest))

	for caseID, version := range latest {
		var skills []string

		if err := json.Unmarshal(version.SkillIDs, &skills); err != nil {
			skills = []string{}
		}

		view := CaseView{
			Key:            keys[caseID],
			Title:          version.Title,
			Mode:           version.Mode,
			Difficulty:     version.Difficulty,
			SkillIDs:       skills,
			MaxTurns:       version.MaxTurns,
			Briefing:       version.Briefing,
			HintsAvailable: version.Mode == modePractice,
		}

		if module, ok := modules[version.ID]; ok {
			view.UnlockedBy = module.Title
			view.Locked = !slices.Contains(assessmentUnlockedBy, statuses[module.ID])
		} else if version.Mode == modeAssessment {
			view.Locked = true
		}

		views = append(views, view)
	}

	slices.SortFunc(views, func(a, b CaseView) int { return strings.Compare(a.Key, b.Key) })

	return views, nil
}

func (s *Service) GetCase(ctx context.Context, companyID, userID, caseKey string) (CaseView, error) {
	const op = "negotiationService.GetCase"

	log := s.log.With(slog.String("op", op), slog.String("company_id", companyID), slog.String("case_key", caseKey))

	cases, err := s.ListCases(ctx, companyID, userID)
	if err != nil {
		return CaseView{}, fmt.Errorf("%s: %w", op, err)
	}

	for _, item := range cases {
		if item.Key == caseKey {
			return item, nil
		}
	}

	log.WarnContext(ctx, "case is not available for this company")

	return CaseView{}, fmt.Errorf("%s: %w", op, ErrNotFound)
}

func (s *Service) StartSession(ctx context.Context, companyID, userID, caseKey string) (SessionView, error) {
	const op = "negotiationService.StartSession"

	log := s.log.With(
		slog.String("op", op),
		slog.String("company_id", companyID),
		slog.String("user_id", userID),
		slog.String("case_key", caseKey),
	)

	db := s.db.WithContext(ctx)

	companyID, userID, err := s.actor(companyID, userID)
	if err != nil {
		log.WarnContext(ctx, "session start rejected: no company or user in context")

		return SessionView{}, fmt.Errorf("%s: %w", op, err)
	}

	scenario, caseVersion, err := s.currentCaseVersion(ctx, db, companyID, caseKey)
	if err != nil {
		return SessionView{}, fmt.Errorf("%s: %w", op, err)
	}

	if caseVersion.Mode != modePractice {
		log.WarnContext(ctx, "session start rejected: case is not a practice one", slog.String("mode", caseVersion.Mode))

		return SessionView{}, fmt.Errorf("%s: %w", op, ErrInvalid)
	}

	session, err := s.createSession(ctx, db, companyID, userID, scenario, caseVersion, nil)
	if err != nil {
		return SessionView{}, fmt.Errorf("%s: %w", op, err)
	}

	return session, nil
}

func (s *Service) StartAssessment(ctx context.Context, companyID, userID, moduleVersionID string) (SessionView, error) {
	const op = "negotiationService.StartAssessment"

	log := s.log.With(
		slog.String("op", op),
		slog.String("company_id", companyID),
		slog.String("user_id", userID),
		slog.String("module_version_id", moduleVersionID),
	)

	if _, err := uuid.Parse(moduleVersionID); err != nil {
		log.WarnContext(ctx, "assessment rejected: module id is not a uuid")

		return SessionView{}, fmt.Errorf("%s: %w", op, ErrNotFound)
	}

	db := s.db.WithContext(ctx)
	if _, _, err := s.actor(companyID, userID); err != nil {
		log.WarnContext(ctx, "assessment rejected: no company or user in context")

		return SessionView{}, fmt.Errorf("%s: %w", op, err)
	}

	var module model.ModuleVersion

	err := db.Where("id = ?", moduleVersionID).Take(&module).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		log.WarnContext(ctx, "assessment rejected: module version is unknown")

		return SessionView{}, fmt.Errorf("%s: %w", op, ErrNotFound)
	}

	if err != nil {
		log.ErrorContext(ctx, "select module version", slog.Any("error", err))

		return SessionView{}, fmt.Errorf("%s: %w", op, err)
	}

	var latest model.ModuleVersion

	if err := db.Where("module_id = ?", module.ModuleID).Order("version DESC").Take(&latest).Error; err != nil {
		log.ErrorContext(ctx, "select latest module version", slog.Any("error", err))

		return SessionView{}, fmt.Errorf("%s: %w", op, err)
	}

	if latest.ID != module.ID {
		log.WarnContext(ctx, "assessment rejected: module version is outdated")

		return SessionView{}, fmt.Errorf("%s: %w", op, ErrNotFound)
	}

	var progress model.ModuleProgress

	err = db.Where("company_id = ? AND user_id = ? AND module_version_id = ?", companyID, userID, module.ID).Take(&progress).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.ErrorContext(ctx, "select module progress", slog.Any("error", err))

		return SessionView{}, fmt.Errorf("%s: %w", op, err)
	}

	if !slices.Contains([]string{progressReady, progressNeedsRetry}, progress.Status) {
		log.WarnContext(ctx, "assessment is locked", slog.String("module_status", progress.Status))

		return SessionView{}, fmt.Errorf("%s: %w", op, ErrLocked)
	}

	scenario, version, err := s.caseVersionByID(ctx, db, module.AssessmentCaseVersionID)
	if err != nil {
		return SessionView{}, fmt.Errorf("%s: %w", op, err)
	}

	if version.Mode != modeAssessment || (scenario.CompanyID != nil && *scenario.CompanyID != companyID) {
		log.WarnContext(ctx, "assessment case is not available for this company")

		return SessionView{}, fmt.Errorf("%s: %w", op, ErrNotFound)
	}

	session, err := s.createSession(ctx, db, companyID, userID, scenario, version, &module.ID)
	if err != nil {
		return SessionView{}, fmt.Errorf("%s: %w", op, err)
	}

	return session, nil
}

func (s *Service) RetrySession(ctx context.Context, companyID, userID, sessionID string) (SessionView, error) {
	const op = "negotiationService.RetrySession"

	log := s.log.With(
		slog.String("op", op),
		slog.String("company_id", companyID),
		slog.String("user_id", userID),
		slog.String("session_id", sessionID),
	)

	db := s.db.WithContext(ctx)

	session, err := s.loadSession(ctx, db, sessionID, companyID, userID)
	if err != nil {
		return SessionView{}, fmt.Errorf("%s: %w", op, err)
	}

	if session.Status == statusActive {
		log.WarnContext(ctx, "retry rejected: current attempt is still active")

		return SessionView{}, fmt.Errorf("%s: %w: finish the current attempt before starting a new one", op, ErrConflict)
	}

	if session.Mode == modeAssessment && session.ModuleVersionID != nil {
		var progress model.ModuleProgress

		if err := db.Where("company_id = ? AND user_id = ? AND module_version_id = ?", companyID, userID, *session.ModuleVersionID).Take(&progress).Error; err != nil {
			log.ErrorContext(ctx, "select module progress", slog.Any("error", err))

			return SessionView{}, fmt.Errorf("%s: %w", op, err)
		}

		if progress.Status == progressPassed {
			log.WarnContext(ctx, "retry rejected: module is already passed")

			return SessionView{}, fmt.Errorf("%s: %w", op, ErrConflict)
		}
	}

	scenario, caseVersion, err := s.caseVersionByID(ctx, db, session.CaseVersionID)
	if err != nil {
		return SessionView{}, fmt.Errorf("%s: %w", op, err)
	}

	retried, err := s.createSession(ctx, db, session.CompanyID, session.UserID, scenario, caseVersion, session.ModuleVersionID)
	if err != nil {
		return SessionView{}, fmt.Errorf("%s: %w", op, err)
	}

	return retried, nil
}

func (s *Service) createSession(
	ctx context.Context,
	db *gorm.DB,
	companyID, userID string,
	scenario model.Case,
	caseVersion model.CaseVersion,
	moduleVersionID *string,
) (SessionView, error) {
	const op = "negotiationService.createSession"

	log := s.log.With(
		slog.String("op", op),
		slog.String("company_id", companyID),
		slog.String("user_id", userID),
		slog.String("case_key", scenario.Key),
	)

	var attempts int64

	err := db.Model(&model.Session{}).
		Where("company_id = ? AND user_id = ? AND case_version_id = ?", companyID, userID, caseVersion.ID).
		Count(&attempts).Error
	if err != nil {
		log.ErrorContext(ctx, "count previous attempts", slog.Any("error", err))

		return SessionView{}, fmt.Errorf("%s: %w", op, err)
	}

	sessionID := uuid.New().String()
	opening := s.reply(ctx, openingRequest(scenario.Key, caseVersion))

	session := model.Session{
		ID:              sessionID,
		CompanyID:       companyID,
		UserID:          userID,
		CaseVersionID:   caseVersion.ID,
		ModuleVersionID: moduleVersionID,
		Mode:            caseVersion.Mode,
		Status:          statusActive,
		StateJSON:       []byte(`{}`),
		Version:         1,
		CreatedAt:       time.Now().UTC(),
	}

	openingMessage := newMessage(sessionID, "", 1, roleOpponent, opening)

	err = s.db.WithContext(context.WithoutCancel(ctx)).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&session).Error; err != nil {
			return fmt.Errorf("create session: %w", err)
		}

		if err := tx.Create(&openingMessage).Error; err != nil {
			return fmt.Errorf("store opening message: %w", err)
		}

		return nil
	})
	if err != nil {
		log.ErrorContext(ctx, "store new session", slog.String("session_id", sessionID), slog.Any("error", err))

		return SessionView{}, fmt.Errorf("%s: %w", op, err)
	}

	log.InfoContext(ctx, "session started",
		slog.String("session_id", sessionID),
		slog.String("mode", session.Mode),
		slog.Int("attempt", int(attempts)+1),
	)

	return SessionView{
		SessionID: sessionID,
		Status:    session.Status,
		Mode:      session.Mode,
		CaseKey:   scenario.Key,
		Title:     caseVersion.Title,
		Briefing:  caseVersion.Briefing,
		Attempt:   int(attempts) + 1,
		TurnsUsed: 0,
		MaxTurns:  caseVersion.MaxTurns,
		HintsUsed: 0,
		HintsLeft: s.hintsLeft(session, 0),
		Messages:  []MessageView{view(openingMessage)},
	}, nil
}

func (s *Service) SendMessage(ctx context.Context, companyID, userID, sessionID, clientMessageID, text string) (TurnView, error) {
	const op = "negotiationService.SendMessage"

	log := s.log.With(
		slog.String("op", op),
		slog.String("company_id", companyID),
		slog.String("user_id", userID),
		slog.String("session_id", sessionID),
	)

	text = strings.TrimSpace(text)
	if text == "" || clientMessageID == "" || len(clientMessageID) > 128 {
		log.WarnContext(ctx, "message rejected: empty text or invalid client_message_id")

		return TurnView{}, fmt.Errorf("%s: %w: message text and client_message_id are required", op, ErrInvalid)
	}

	db := s.db.WithContext(ctx)

	session, err := s.loadSession(ctx, db, sessionID, companyID, userID)
	if err != nil {
		return TurnView{}, fmt.Errorf("%s: %w", op, err)
	}

	scenario, caseVersion, err := s.caseVersionByID(ctx, db, session.CaseVersionID)
	if err != nil {
		return TurnView{}, fmt.Errorf("%s: %w", op, err)
	}

	replayed, found, err := s.replayTurn(ctx, db, session, caseVersion, clientMessageID, text)
	if err != nil {
		return TurnView{}, fmt.Errorf("%s: %w", op, err)
	}

	if found {
		log.DebugContext(ctx, "turn replayed from storage", slog.Int("turn_number", replayed.TurnNumber))

		return replayed, nil
	}

	if session.Status != statusActive {
		log.WarnContext(ctx, "message rejected: session is not active", slog.String("status", session.Status))

		return TurnView{}, fmt.Errorf("%s: %w", op, ErrConflict)
	}

	history, err := s.history(ctx, db, sessionID)
	if err != nil {
		return TurnView{}, fmt.Errorf("%s: %w", op, err)
	}

	turnNumber := 1

	for _, message := range history {
		if message.TurnNumber >= turnNumber {
			turnNumber = message.TurnNumber + 1
		}
	}

	turnsUsed := playerTurns(history) + 1
	answer := s.reply(ctx, dialogueRequest(scenario.Key, caseVersion, history, text, turnsUsed))

	playerMessage := newMessage(sessionID, clientMessageID, turnNumber, roleUser, text)
	opponentMessage := newMessage(sessionID, "", turnNumber, roleOpponent, answer)

	status := session.Status

	if turnsUsed >= caseVersion.MaxTurns {
		status = statusFinished
	}

	err = s.db.WithContext(context.WithoutCancel(ctx)).Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{"version": session.Version + 1}
		if status == statusFinished {
			updates["status"] = statusFinished
			updates["finished_at"] = time.Now().UTC()
		}

		update := tx.Model(&model.Session{}).Where("id = ? AND version = ? AND status = ?", sessionID, session.Version, statusActive).Updates(updates)
		if update.Error != nil {
			return update.Error
		}

		if update.RowsAffected != 1 {
			return ErrConflict
		}

		if err := tx.Create(&playerMessage).Error; err != nil {
			return fmt.Errorf("store player message: %w", err)
		}

		if err := tx.Create(&opponentMessage).Error; err != nil {
			return fmt.Errorf("store opponent message: %w", err)
		}

		return nil
	})
	if err != nil {
		if errors.Is(err, ErrConflict) {
			current, loadErr := s.loadSession(ctx, db, sessionID, companyID, userID)
			if loadErr == nil {
				replayed, found, replayErr := s.replayTurn(ctx, db, current, caseVersion, clientMessageID, text)
				if found || replayErr != nil {
					if replayErr != nil {
						return TurnView{}, fmt.Errorf("%s: %w", op, replayErr)
					}

					return replayed, nil
				}
			}
		}

		log.ErrorContext(ctx, "store turn", slog.Int("turn_number", turnNumber), slog.Any("error", err))

		return TurnView{}, fmt.Errorf("%s: %w", op, err)
	}

	log.InfoContext(ctx, "turn stored",
		slog.Int("turn_number", turnNumber),
		slog.Int("turns_used", turnsUsed),
		slog.String("status", status),
	)

	return TurnView{
		SessionID:     sessionID,
		Status:        status,
		TurnNumber:    turnNumber,
		PlayerMessage: view(playerMessage),
		Opponent:      view(opponentMessage),
		TurnsUsed:     turnsUsed,
		MaxTurns:      caseVersion.MaxTurns,
	}, nil
}

func (s *Service) Hint(ctx context.Context, companyID, userID, sessionID string) (HintView, error) {
	const op = "negotiationService.Hint"

	log := s.log.With(
		slog.String("op", op),
		slog.String("company_id", companyID),
		slog.String("user_id", userID),
		slog.String("session_id", sessionID),
	)

	db := s.db.WithContext(ctx)

	session, err := s.loadSession(ctx, db, sessionID, companyID, userID)
	if err != nil {
		return HintView{}, fmt.Errorf("%s: %w", op, err)
	}

	if session.Mode != modePractice {
		log.WarnContext(ctx, "hint rejected: assessment session", slog.String("mode", session.Mode))

		return HintView{}, fmt.Errorf("%s: %w: hints are disabled in assessment cases", op, ErrForbidden)
	}

	if session.Status != statusActive {
		log.WarnContext(ctx, "hint rejected: session is not active", slog.String("status", session.Status))

		return HintView{}, fmt.Errorf("%s: %w", op, ErrConflict)
	}

	state := parseState(session.StateJSON)
	if state.HintsUsed >= s.cfg.HintLimit {
		log.WarnContext(ctx, "hint rejected: limit is reached", slog.Int("hints_used", state.HintsUsed))

		return HintView{}, fmt.Errorf("%s: %w: hint limit of %d is reached", op, ErrConflict, s.cfg.HintLimit)
	}

	scenario, caseVersion, err := s.caseVersionByID(ctx, db, session.CaseVersionID)
	if err != nil {
		return HintView{}, fmt.Errorf("%s: %w", op, err)
	}

	rubric, err := parseRubric(caseVersion.Rubric)
	if err != nil {
		log.ErrorContext(ctx, "parse case rubric", slog.Any("error", err))

		return HintView{}, fmt.Errorf("%s: %w", op, err)
	}

	messages, err := s.history(ctx, db, sessionID)
	if err != nil {
		return HintView{}, fmt.Errorf("%s: %w", op, err)
	}

	hintCtx, cancel := context.WithTimeout(ctx, s.cfg.ReplyTimeout)

	defer cancel()

	text, err := s.provider.Complete(hintCtx, hintRequest(scenario.Key, caseVersion, rubric, messages))
	if err != nil {
		log.ErrorContext(ctx, "hint degraded to the static advice", slog.Any("error", err))

		return HintView{
			SessionID: sessionID,
			Hint:      fallbackHint,
			HintsUsed: state.HintsUsed,
			HintsLeft: s.cfg.HintLimit - state.HintsUsed,
		}, nil
	}

	state.HintsUsed++

	encoded, err := json.Marshal(state)
	if err != nil {
		log.ErrorContext(ctx, "encode session state", slog.Any("error", err))

		return HintView{}, fmt.Errorf("%s: encode session state: %w", op, err)
	}

	update := db.Model(&model.Session{}).Where("id = ? AND version = ?", sessionID, session.Version).
		Updates(map[string]any{"state_json": encoded, "version": session.Version + 1})
	if update.Error != nil {
		log.ErrorContext(ctx, "store used hint", slog.Any("error", update.Error))

		return HintView{}, fmt.Errorf("%s: %w", op, update.Error)
	}

	if update.RowsAffected == 0 {
		log.WarnContext(ctx, "hint rejected: session changed while the hint was prepared")

		return HintView{}, fmt.Errorf("%s: %w: session changed while the hint was prepared", op, ErrConflict)
	}

	log.InfoContext(ctx, "hint issued", slog.Int("hints_used", state.HintsUsed))

	return HintView{
		SessionID: sessionID,
		Hint:      strings.TrimSpace(text),
		HintsUsed: state.HintsUsed,
		HintsLeft: s.cfg.HintLimit - state.HintsUsed,
	}, nil
}

func (s *Service) GetSession(ctx context.Context, companyID, userID, sessionID string) (SessionView, error) {
	const op = "negotiationService.GetSession"

	db := s.db.WithContext(ctx)

	session, err := s.loadSession(ctx, db, sessionID, companyID, userID)
	if err != nil {
		return SessionView{}, fmt.Errorf("%s: %w", op, err)
	}

	scenario, caseVersion, err := s.caseVersionByID(ctx, db, session.CaseVersionID)
	if err != nil {
		return SessionView{}, fmt.Errorf("%s: %w", op, err)
	}

	messages, err := s.history(ctx, db, sessionID)
	if err != nil {
		return SessionView{}, fmt.Errorf("%s: %w", op, err)
	}

	views := make([]MessageView, 0, len(messages))

	for _, message := range messages {
		views = append(views, view(message))
	}

	attempt, err := s.attemptNumber(ctx, db, session)
	if err != nil {
		return SessionView{}, fmt.Errorf("%s: %w", op, err)
	}

	state := parseState(session.StateJSON)

	return SessionView{
		SessionID: session.ID,
		Status:    session.Status,
		Mode:      session.Mode,
		CaseKey:   scenario.Key,
		Title:     caseVersion.Title,
		Briefing:  caseVersion.Briefing,
		Attempt:   attempt,
		TurnsUsed: playerTurns(messages),
		MaxTurns:  caseVersion.MaxTurns,
		HintsUsed: state.HintsUsed,
		HintsLeft: s.hintsLeft(session, state.HintsUsed),
		Messages:  views,
	}, nil
}

func (s *Service) FinishSession(ctx context.Context, companyID, userID, sessionID string) (ResultView, error) {
	const op = "negotiationService.FinishSession"

	log := s.log.With(
		slog.String("op", op),
		slog.String("company_id", companyID),
		slog.String("user_id", userID),
		slog.String("session_id", sessionID),
	)

	db := s.db.WithContext(ctx)

	session, err := s.loadSession(ctx, db, sessionID, companyID, userID)
	if err != nil {
		return ResultView{}, fmt.Errorf("%s: %w", op, err)
	}

	if session.Status == statusActive {
		finishedAt := time.Now().UTC()

		update := db.Model(&model.Session{}).Where("id = ? AND version = ? AND status = ?", sessionID, session.Version, statusActive).
			Updates(map[string]any{"status": statusFinished, "finished_at": finishedAt, "version": session.Version + 1})
		if update.Error != nil {
			log.ErrorContext(ctx, "finish session", slog.Any("error", update.Error))

			return ResultView{}, fmt.Errorf("%s: %w", op, update.Error)
		}

		if update.RowsAffected == 0 {
			session, err = s.loadSession(ctx, db, sessionID, companyID, userID)
			if err != nil || session.Status == statusActive {
				log.WarnContext(ctx, "finish rejected: session changed concurrently", slog.Any("error", err))

				return ResultView{}, fmt.Errorf("%s: %w", op, ErrConflict)
			}
		} else {
			session.Status = statusFinished

			log.InfoContext(ctx, "session finished")
		}
	}

	result, err := s.result(ctx, db, session)
	if err != nil {
		return ResultView{}, fmt.Errorf("%s: %w", op, err)
	}

	return result, nil
}

func (s *Service) GetResult(ctx context.Context, companyID, userID, sessionID string) (ResultView, error) {
	const op = "negotiationService.GetResult"

	log := s.log.With(
		slog.String("op", op),
		slog.String("company_id", companyID),
		slog.String("user_id", userID),
		slog.String("session_id", sessionID),
	)

	db := s.db.WithContext(ctx)

	session, err := s.loadSession(ctx, db, sessionID, companyID, userID)
	if err != nil {
		return ResultView{}, fmt.Errorf("%s: %w", op, err)
	}

	if session.Status == statusActive {
		log.WarnContext(ctx, "result rejected: session is still active")

		return ResultView{}, fmt.Errorf("%s: %w: session is still active", op, ErrConflict)
	}

	result, err := s.result(ctx, db, session)
	if err != nil {
		return ResultView{}, fmt.Errorf("%s: %w", op, err)
	}

	return result, nil
}

func (s *Service) result(ctx context.Context, db *gorm.DB, session model.Session) (ResultView, error) {
	const op = "negotiationService.result"

	log := s.log.With(slog.String("op", op), slog.String("session_id", session.ID))

	scenario, caseVersion, err := s.caseVersionByID(ctx, db, session.CaseVersionID)
	if err != nil {
		return ResultView{}, fmt.Errorf("%s: %w", op, err)
	}

	rubric, err := parseRubric(caseVersion.Rubric)
	if err != nil {
		log.ErrorContext(ctx, "parse case rubric", slog.Any("error", err))

		return ResultView{}, fmt.Errorf("%s: %w", op, err)
	}

	messages, err := s.history(ctx, db, session.ID)
	if err != nil {
		return ResultView{}, fmt.Errorf("%s: %w", op, err)
	}

	assessment, err := s.assessment(ctx, db, session, scenario, caseVersion, rubric, messages)
	if err != nil {
		return ResultView{}, fmt.Errorf("%s: %w", op, err)
	}

	moduleStatus := ""
	if assessment.EvaluationStatus == evaluationCompleted {
		err = db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", session.ID).Take(&session).Error; err != nil {
				return err
			}

			if session.Mode == modeAssessment {
				if session.ModuleVersionID == nil || assessment.RubricVersion != rubric.Version {
					return ErrConflict
				}

				var module model.ModuleVersion

				if err := tx.Where("id = ? AND assessment_case_version_id = ?", *session.ModuleVersionID, session.CaseVersionID).Take(&module).Error; err != nil {
					return ErrConflict
				}
			}

			var stored model.Assessment

			findErr := tx.Where("session_id = ?", session.ID).Take(&stored).Error
			if errors.Is(findErr, gorm.ErrRecordNotFound) {
				if err := tx.Create(&assessment).Error; err != nil {
					return err
				}
			} else if findErr != nil {
				return findErr
			} else {
				assessment = stored
			}

			if session.Status == statusFinished {
				if err := tx.Model(&session).Update("status", statusEvaluated).Error; err != nil {
					return err
				}

				session.Status = statusEvaluated
			}

			var syncErr error

			moduleStatus, syncErr = s.syncModuleProgress(ctx, tx, session, assessment)

			return syncErr
		})
		if err != nil {
			if errors.Is(err, ErrConflict) {
				log.WarnContext(ctx, "assessment was not stored: session changed concurrently")
			} else {
				log.ErrorContext(ctx, "store assessment", slog.Any("error", err))
			}

			return ResultView{}, fmt.Errorf("%s: %w", op, err)
		}
	} else if session.Mode == modeAssessment {
		moduleStatus = progressReady
		if session.ModuleVersionID != nil {
			var progress model.ModuleProgress

			findErr := db.Where("company_id = ? AND user_id = ? AND module_version_id = ?", session.CompanyID, session.UserID, *session.ModuleVersionID).Take(&progress).Error
			if findErr == nil {
				moduleStatus = progress.Status
			} else if !errors.Is(findErr, gorm.ErrRecordNotFound) {
				log.ErrorContext(ctx, "select module progress", slog.Any("error", findErr))

				return ResultView{}, fmt.Errorf("%s: %w", op, findErr)
			}
		}
	}

	var criteria []CriterionView

	if err := json.Unmarshal(assessment.CriteriaJSON, &criteria); err != nil {
		log.ErrorContext(ctx, "decode stored criteria", slog.Any("error", err))

		return ResultView{}, fmt.Errorf("%s: decode stored criteria: %w", op, err)
	}

	var feedback Feedback

	if err := json.Unmarshal(assessment.FeedbackJSON, &feedback); err != nil {
		log.ErrorContext(ctx, "decode stored feedback", slog.Any("error", err))

		return ResultView{}, fmt.Errorf("%s: decode stored feedback: %w", op, err)
	}

	attempt, err := s.attemptNumber(ctx, db, session)
	if err != nil {
		return ResultView{}, fmt.Errorf("%s: %w", op, err)
	}

	return ResultView{
		SessionID:        session.ID,
		Status:           session.Status,
		Mode:             session.Mode,
		CaseKey:          scenario.Key,
		Title:            caseVersion.Title,
		Passed:           assessment.Passed,
		EvaluationStatus: assessment.EvaluationStatus,
		ModuleStatus:     moduleStatus,
		RubricVersion:    assessment.RubricVersion,
		Criteria:         criteria,
		Feedback:         feedback,
		Attempt:          attempt,
		TurnsUsed:        playerTurns(messages),
		MaxTurns:         caseVersion.MaxTurns,
		CanRetry:         assessment.EvaluationStatus == evaluationCompleted && !assessment.Passed,
	}, nil
}

func (s *Service) assessment(
	ctx context.Context,
	db *gorm.DB,
	session model.Session,
	scenario model.Case,
	caseVersion model.CaseVersion,
	rubric rubric,
	messages []model.Message,
) (model.Assessment, error) {
	const op = "negotiationService.assessment"

	log := s.log.With(slog.String("op", op), slog.String("session_id", session.ID))

	var stored model.Assessment

	err := db.First(&stored, "session_id = ?", session.ID).Error
	if err == nil {
		return stored, nil
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		log.ErrorContext(ctx, "select stored assessment", slog.Any("error", err))

		return model.Assessment{}, fmt.Errorf("%s: %w", op, err)
	}

	criteria, feedback, status := s.judge(ctx, scenario.Key, caseVersion, rubric, messages)

	criteriaJSON, err := json.Marshal(criteria)
	if err != nil {
		log.ErrorContext(ctx, "encode criteria", slog.Any("error", err))

		return model.Assessment{}, fmt.Errorf("%s: encode criteria: %w", op, err)
	}

	feedbackJSON, err := json.Marshal(feedback)
	if err != nil {
		log.ErrorContext(ctx, "encode feedback", slog.Any("error", err))

		return model.Assessment{}, fmt.Errorf("%s: encode feedback: %w", op, err)
	}

	fresh := model.Assessment{
		ID:               uuid.New().String(),
		SessionID:        session.ID,
		RubricVersion:    rubric.Version,
		CriteriaJSON:     criteriaJSON,
		Passed:           passed(criteria),
		FeedbackJSON:     feedbackJSON,
		EvaluationStatus: status,
		CreatedAt:        time.Now().UTC(),
	}

	return fresh, nil
}

func (s *Service) judge(
	ctx context.Context,
	caseKey string,
	caseVersion model.CaseVersion,
	rubric rubric,
	messages []model.Message,
) ([]CriterionView, Feedback, string) {
	const op = "negotiationService.judge"

	log := s.log.With(slog.String("op", op), slog.String("case_key", caseKey))

	judgeCtx, cancel := context.WithTimeout(ctx, s.cfg.AssessmentTimeout)

	defer cancel()

	request := assessmentRequest(caseKey, rubric, caseVersion, messages)

	answer, err := s.provider.Complete(judgeCtx, request)
	if err != nil {
		log.ErrorContext(ctx, "assessment degraded to unmet rubric", slog.Any("error", err))

		criteria, feedback := degradedVerdict(rubric)

		return criteria, feedback, evaluationDegraded
	}

	criteria, feedback, err := parseVerdict(answer, rubric, request.PlayerLines)
	if err != nil {
		log.WarnContext(ctx, "assessment verdict is not valid JSON, retrying",
			slog.Any("error", err),
			slog.String("answer", truncate(answer, 800)),
		)

		request.Messages = append(request.Messages, llm.Message{
			Role:    llm.RoleUser,
			Content: "Повтори оценку по той же стенограмме. Предыдущий ответ не удалось разобрать. Верни только корректный JSON-объект с полями criteria, what_worked и what_to_try_next. В criteria перечисли каждый критерий из задания.",
		})

		answer, retryErr := s.provider.Complete(judgeCtx, request)
		if retryErr == nil {
			criteria, feedback, retryErr = parseVerdict(answer, rubric, request.PlayerLines)
		}

		if retryErr == nil {
			return criteria, feedback, evaluationCompleted
		}

		log.ErrorContext(ctx, "assessment verdict retry failed",
			slog.Any("error", retryErr),
			slog.String("answer", truncate(answer, 800)),
		)

		criteria, feedback := degradedVerdict(rubric)

		return criteria, feedback, evaluationDegraded
	}

	return criteria, feedback, evaluationCompleted
}

func (s *Service) syncModuleProgress(ctx context.Context, db *gorm.DB, session model.Session, assessment model.Assessment) (string, error) {
	const op = "negotiationService.syncModuleProgress"

	log := s.log.With(slog.String("op", op), slog.String("session_id", session.ID))

	if session.Mode != modeAssessment || session.ModuleVersionID == nil {
		return "", nil
	}

	if assessment.EvaluationStatus != evaluationCompleted {
		return progressReady, nil
	}

	status := progressNeedsRetry

	var passedSessionID *string

	if assessment.Passed {
		status = progressPassed
		passedSessionID = &session.ID
	}

	var result struct{ Status string }

	err := db.Raw(`INSERT INTO module_progress (company_id,user_id,module_version_id,status,passed_session_id,updated_at)
		VALUES (?,?,?,?,?,now())
		ON CONFLICT (company_id,user_id,module_version_id) DO UPDATE SET
		status = CASE WHEN module_progress.status = 'passed' THEN 'passed' ELSE EXCLUDED.status END,
		passed_session_id = CASE WHEN module_progress.status = 'passed' THEN module_progress.passed_session_id ELSE EXCLUDED.passed_session_id END,
		updated_at = now()
		RETURNING status`, session.CompanyID, session.UserID, *session.ModuleVersionID, status, passedSessionID).Scan(&result).Error
	if err != nil {
		log.ErrorContext(ctx, "store module progress", slog.Any("error", err))

		return "", fmt.Errorf("%s: store module progress: %w", op, err)
	}

	log.InfoContext(ctx, "module progress updated",
		slog.String("module_version_id", *session.ModuleVersionID),
		slog.String("module_status", result.Status),
	)

	return result.Status, nil
}

func (s *Service) reply(ctx context.Context, request llm.Request) string {
	const op = "negotiationService.reply"

	log := s.log.With(slog.String("op", op), slog.String("case_key", request.CaseKey))

	replyCtx, cancel := context.WithTimeout(ctx, s.cfg.ReplyTimeout)

	defer cancel()

	text, err := s.provider.Complete(replyCtx, request)
	if err != nil {
		log.ErrorContext(ctx, "opponent reply degraded to fallback", slog.Any("error", err))

		return fallbackReply
	}

	return text
}

func (s *Service) replayTurn(ctx context.Context, db *gorm.DB, session model.Session, caseVersion model.CaseVersion, clientMessageID, text string) (TurnView, bool, error) {
	const op = "negotiationService.replayTurn"

	log := s.log.With(slog.String("op", op), slog.String("session_id", session.ID))

	var player model.Message

	err := db.Where("session_id = ? AND client_message_id = ?", session.ID, clientMessageID).First(&player).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return TurnView{}, false, nil
	}

	if err != nil {
		log.ErrorContext(ctx, "select stored player message", slog.Any("error", err))

		return TurnView{}, false, fmt.Errorf("%s: %w", op, err)
	}

	if player.Text != text {
		log.WarnContext(ctx, "replay rejected: client_message_id is reused with another text")

		return TurnView{}, true, fmt.Errorf("%s: %w", op, ErrConflict)
	}

	var opponent model.Message

	err = db.Where("session_id = ? AND turn_number = ? AND role = ?", session.ID, player.TurnNumber, roleOpponent).First(&opponent).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		log.ErrorContext(ctx, "stored turn has no opponent reply", slog.Int("turn_number", player.TurnNumber))

		return TurnView{}, false, fmt.Errorf("%s: stored turn has no opponent reply: %w", op, err)
	}

	if err != nil {
		log.ErrorContext(ctx, "select stored opponent message", slog.Any("error", err))

		return TurnView{}, false, fmt.Errorf("%s: %w", op, err)
	}

	var turnsUsed int64

	if err := db.Model(&model.Message{}).Where("session_id = ? AND role = ? AND turn_number <= ?", session.ID, roleUser, player.TurnNumber).Count(&turnsUsed).Error; err != nil {
		log.ErrorContext(ctx, "count used turns", slog.Any("error", err))

		return TurnView{}, false, fmt.Errorf("%s: %w", op, err)
	}

	status := statusActive
	if int(turnsUsed) >= caseVersion.MaxTurns {
		status = statusFinished
	}

	return TurnView{
		SessionID:     session.ID,
		Status:        status,
		TurnNumber:    player.TurnNumber,
		PlayerMessage: view(player),
		Opponent:      view(opponent),
		TurnsUsed:     int(turnsUsed),
		MaxTurns:      caseVersion.MaxTurns,
	}, true, nil
}

func (s *Service) loadSession(ctx context.Context, db *gorm.DB, sessionID, companyID, userID string) (model.Session, error) {
	const op = "negotiationService.loadSession"

	log := s.log.With(
		slog.String("op", op),
		slog.String("session_id", sessionID),
		slog.String("company_id", companyID),
		slog.String("user_id", userID),
	)

	if _, err := uuid.Parse(sessionID); err != nil {
		log.WarnContext(ctx, "session id is not a uuid")

		return model.Session{}, fmt.Errorf("%s: %w", op, ErrNotFound)
	}

	var session model.Session

	err := db.First(&session, "id = ? AND company_id = ? AND user_id = ?", sessionID, companyID, userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		log.WarnContext(ctx, "session is not found for this company and user")

		return model.Session{}, fmt.Errorf("%s: %w", op, ErrNotFound)
	}

	if err != nil {
		log.ErrorContext(ctx, "select session", slog.Any("error", err))

		return model.Session{}, fmt.Errorf("%s: %w", op, err)
	}

	return session, nil
}

func (s *Service) history(ctx context.Context, db *gorm.DB, sessionID string) ([]model.Message, error) {
	const op = "negotiationService.history"

	log := s.log.With(slog.String("op", op), slog.String("session_id", sessionID))

	var messages []model.Message

	err := db.Where("session_id = ?", sessionID).Order("turn_number, role DESC").Find(&messages).Error
	if err != nil {
		log.ErrorContext(ctx, "select session messages", slog.Any("error", err))

		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return messages, nil
}

func (s *Service) attemptNumber(ctx context.Context, db *gorm.DB, session model.Session) (int, error) {
	const op = "negotiationService.attemptNumber"

	log := s.log.With(slog.String("op", op), slog.String("session_id", session.ID))

	var earlier int64

	err := db.Model(&model.Session{}).
		Where("company_id = ? AND user_id = ? AND case_version_id = ? AND created_at <= ?",
			session.CompanyID, session.UserID, session.CaseVersionID, session.CreatedAt).
		Count(&earlier).Error
	if err != nil {
		log.ErrorContext(ctx, "count session attempts", slog.Any("error", err))

		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return int(earlier), nil
}

func (s *Service) hintsLeft(session model.Session, used int) int {
	if session.Mode != modePractice {
		return 0
	}

	if used >= s.cfg.HintLimit {
		return 0
	}

	return s.cfg.HintLimit - used
}

func (s *Service) actor(companyID, userID string) (string, string, error) {
	if companyID != "" && userID != "" {
		return companyID, userID, nil
	}

	return "", "", ErrForbidden
}

func (s *Service) currentCaseVersion(ctx context.Context, db *gorm.DB, companyID, caseKey string) (model.Case, model.CaseVersion, error) {
	const op = "negotiationService.currentCaseVersion"

	if caseKey == "" {
		caseKey = s.cfg.DefaultCaseKey
	}

	log := s.log.With(slog.String("op", op), slog.String("company_id", companyID), slog.String("case_key", caseKey))

	var scenario model.Case

	err := db.Where("key = ? AND (company_id IS NULL OR company_id = ?)", caseKey, companyID).First(&scenario).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		log.WarnContext(ctx, "scenario is not published")

		return model.Case{}, model.CaseVersion{}, fmt.Errorf("%s: %w: scenario %q is not published", op, ErrNotFound, caseKey)
	}

	if err != nil {
		log.ErrorContext(ctx, "select scenario", slog.Any("error", err))

		return model.Case{}, model.CaseVersion{}, fmt.Errorf("%s: %w", op, err)
	}

	var caseVersion model.CaseVersion

	err = db.Where("case_id = ?", scenario.ID).Order("version DESC").First(&caseVersion).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		log.WarnContext(ctx, "scenario has no published version")

		return model.Case{}, model.CaseVersion{}, fmt.Errorf("%s: %w: scenario %q has no published version", op, ErrNotFound, caseKey)
	}

	if err != nil {
		log.ErrorContext(ctx, "select case version", slog.Any("error", err))

		return model.Case{}, model.CaseVersion{}, fmt.Errorf("%s: %w", op, err)
	}

	return scenario, caseVersion, nil
}

func (s *Service) caseVersionByID(ctx context.Context, db *gorm.DB, id string) (model.Case, model.CaseVersion, error) {
	const op = "negotiationService.caseVersionByID"

	log := s.log.With(slog.String("op", op), slog.String("case_version_id", id))

	var caseVersion model.CaseVersion

	err := db.First(&caseVersion, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		log.WarnContext(ctx, "case version is not found")

		return model.Case{}, model.CaseVersion{}, fmt.Errorf("%s: %w", op, ErrNotFound)
	}

	if err != nil {
		log.ErrorContext(ctx, "select case version", slog.Any("error", err))

		return model.Case{}, model.CaseVersion{}, fmt.Errorf("%s: %w", op, err)
	}

	var scenario model.Case

	if err := db.First(&scenario, "id = ?", caseVersion.CaseID).Error; err != nil {
		log.ErrorContext(ctx, "select scenario", slog.Any("error", err))

		return model.Case{}, model.CaseVersion{}, fmt.Errorf("%s: %w", op, err)
	}

	return scenario, caseVersion, nil
}

func assessmentModules(db *gorm.DB) (map[string]model.ModuleVersion, error) {
	var versions []model.ModuleVersion

	if err := db.Order("version").Find(&versions).Error; err != nil {
		return nil, err
	}

	modules := make(map[string]model.ModuleVersion, len(versions))

	for _, version := range versions {
		modules[version.AssessmentCaseVersionID] = version
	}

	return modules, nil
}

func progressStatuses(db *gorm.DB, companyID, userID string) (map[string]string, error) {
	var rows []model.ModuleProgress

	if err := db.Where("company_id = ? AND user_id = ?", companyID, userID).Find(&rows).Error; err != nil {
		return nil, err
	}

	statuses := make(map[string]string, len(rows))

	for _, row := range rows {
		statuses[row.ModuleVersionID] = row.Status
	}

	return statuses, nil
}

func parseState(raw json.RawMessage) sessionState {
	var state sessionState

	if len(raw) == 0 {
		return state
	}

	if err := json.Unmarshal(raw, &state); err != nil {
		return sessionState{}
	}

	return state
}

func promptHistory(messages []model.Message) []llm.Message {
	if len(messages) > historyLimit {
		messages = messages[len(messages)-historyLimit:]
	}

	prompt := make([]llm.Message, 0, len(messages))

	for _, message := range messages {
		role := llm.RoleUser
		if message.Role == roleOpponent {
			role = llm.RoleAssistant
		}

		prompt = append(prompt, llm.Message{Role: role, Content: message.Text})
	}

	return prompt
}

func playerTurns(messages []model.Message) int {
	count := 0

	for _, message := range messages {
		if message.Role == roleUser {
			count++
		}
	}

	return count
}

func newMessage(sessionID, clientMessageID string, turnNumber int, role, text string) model.Message {
	message := model.Message{
		ID:         uuid.New().String(),
		SessionID:  sessionID,
		TurnNumber: turnNumber,
		Role:       role,
		Text:       text,
		CreatedAt:  time.Now().UTC().Truncate(time.Microsecond),
	}

	if clientMessageID != "" {
		message.ClientMessageID = &clientMessageID
	}

	return message
}

func view(message model.Message) MessageView {
	return MessageView{
		Role:       message.Role,
		TurnNumber: message.TurnNumber,
		Text:       message.Text,
		CreatedAt:  message.CreatedAt.UTC(),
	}
}
