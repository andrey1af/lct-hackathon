package model

import (
	"encoding/json"
	"time"
)

type Company struct {
	ID        string `gorm:"type:uuid;primaryKey"`
	Name      string `gorm:"not null"`
	CreatedAt time.Time
}

type Membership struct {
	CompanyID string `gorm:"type:uuid;primaryKey"`
	UserID    string `gorm:"type:uuid;primaryKey"`
	Role      string `gorm:"not null"`
	Status    string `gorm:"not null"`
	CreatedAt time.Time
}

type AuthSession struct {
	ID        string    `gorm:"type:uuid;primaryKey"`
	UserID    string    `gorm:"type:uuid;not null"`
	TokenHash string    `gorm:"not null;uniqueIndex"`
	ExpiresAt time.Time `gorm:"not null"`
	CreatedAt time.Time
}

type Invitation struct {
	ID         string    `gorm:"type:uuid;primaryKey"`
	CompanyID  string    `gorm:"type:uuid;not null"`
	Email      string    `gorm:"not null"`
	Role       string    `gorm:"not null"`
	TokenHash  string    `gorm:"not null;uniqueIndex"`
	ExpiresAt  time.Time `gorm:"not null"`
	AcceptedAt *time.Time
	CreatedAt  time.Time
}

type Case struct {
	ID        string  `gorm:"type:uuid;primaryKey"`
	CompanyID *string `gorm:"type:uuid"`
	Key       string  `gorm:"not null;uniqueIndex"`
	CreatedAt time.Time
}

type CaseVersion struct {
	ID            string          `gorm:"type:uuid;primaryKey"`
	CaseID        string          `gorm:"type:uuid;not null"`
	Version       int             `gorm:"not null"`
	Mode          string          `gorm:"not null"`
	Title         string          `gorm:"not null"`
	Difficulty    string          `gorm:"not null"`
	SkillIDs      json.RawMessage `gorm:"type:jsonb;not null"`
	Briefing      string          `gorm:"not null"`
	PrivateConfig json.RawMessage `gorm:"type:jsonb;not null"`
	Rubric        json.RawMessage `gorm:"type:jsonb;not null"`
	MaxTurns      int             `gorm:"not null"`
	ContentHash   string          `gorm:"not null"`
	PublishedAt   time.Time       `gorm:"not null"`
}

type Module struct {
	ID        string `gorm:"type:uuid;primaryKey"`
	Key       string `gorm:"not null;uniqueIndex"`
	CreatedAt time.Time
}

type ModuleVersion struct {
	ID                      string          `gorm:"type:uuid;primaryKey"`
	ModuleID                string          `gorm:"type:uuid;not null"`
	Version                 int             `gorm:"not null"`
	OrderIndex              int             `gorm:"not null"`
	Title                   string          `gorm:"not null"`
	SkillIDs                json.RawMessage `gorm:"type:jsonb;not null"`
	AssessmentCaseVersionID string          `gorm:"type:uuid;not null"`
	ContentHash             string          `gorm:"not null"`
	PublishedAt             time.Time       `gorm:"not null"`
}

type Step struct {
	ID              string          `gorm:"type:uuid;primaryKey"`
	ModuleVersionID string          `gorm:"type:uuid;not null"`
	OrderIndex      int             `gorm:"not null"`
	Type            string          `gorm:"not null"`
	Content         string          `gorm:"not null"`
	Options         json.RawMessage `gorm:"type:jsonb;not null"`
	AnswerKey       string          `gorm:"not null"`
	Explanation     string          `gorm:"not null"`
}

type LearningAttempt struct {
	ID              string          `gorm:"type:uuid;primaryKey"`
	CompanyID       string          `gorm:"type:uuid;not null"`
	UserID          string          `gorm:"type:uuid;not null"`
	StepID          string          `gorm:"type:uuid;not null"`
	ModuleVersionID string          `gorm:"type:uuid;not null"`
	ClientAttemptID string          `gorm:"not null"`
	Answer          string          `gorm:"not null"`
	Correct         bool            `gorm:"not null"`
	Completed       bool            `gorm:"not null"`
	ResultJSON      json.RawMessage `gorm:"type:jsonb;not null"`
	CreatedAt       time.Time       `gorm:"not null"`
}

type ModuleProgress struct {
	CompanyID       string    `gorm:"type:uuid;primaryKey"`
	UserID          string    `gorm:"type:uuid;primaryKey"`
	ModuleVersionID string    `gorm:"type:uuid;primaryKey"`
	Status          string    `gorm:"not null"`
	PassedSessionID *string   `gorm:"type:uuid"`
	UpdatedAt       time.Time `gorm:"not null"`
}

func (ModuleProgress) TableName() string {
	return "module_progress"
}

type Session struct {
	ID              string          `gorm:"type:uuid;primaryKey"`
	CompanyID       string          `gorm:"type:uuid;not null"`
	UserID          string          `gorm:"type:uuid;not null"`
	CaseVersionID   string          `gorm:"type:uuid;not null"`
	ModuleVersionID *string         `gorm:"type:uuid"`
	Mode            string          `gorm:"not null"`
	Status          string          `gorm:"not null"`
	StateJSON       json.RawMessage `gorm:"type:jsonb;not null"`
	Version         int             `gorm:"not null"`
	CreatedAt       time.Time       `gorm:"not null"`
	FinishedAt      *time.Time
}

type Message struct {
	ID              string `gorm:"type:uuid;primaryKey"`
	SessionID       string `gorm:"type:uuid;not null"`
	ClientMessageID *string
	TurnNumber      int       `gorm:"not null"`
	Role            string    `gorm:"not null"`
	Text            string    `gorm:"not null"`
	CreatedAt       time.Time `gorm:"not null"`
}

type Assessment struct {
	ID               string          `gorm:"type:uuid;primaryKey"`
	SessionID        string          `gorm:"type:uuid;not null;uniqueIndex"`
	RubricVersion    int             `gorm:"not null"`
	CriteriaJSON     json.RawMessage `gorm:"type:jsonb;not null"`
	Passed           bool            `gorm:"not null"`
	FeedbackJSON     json.RawMessage `gorm:"type:jsonb;not null"`
	EvaluationStatus string          `gorm:"not null"`
	CreatedAt        time.Time       `gorm:"not null"`
}
