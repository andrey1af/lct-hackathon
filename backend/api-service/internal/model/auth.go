package model

import (
	"time"
)

type User struct {
	ID           string    `gorm:"type:uuid;primaryKey" json:"id"`
	Email        string    `gorm:"type:text;uniqueIndex;not null" json:"email"`
	PasswordHash string    `gorm:"type:text;not null" json:"-"`
	Name         string    `gorm:"type:text;not null" json:"name"`
	CreatedAt    time.Time `gorm:"type:timestamptz;not null;default:now()" json:"created_at"`
}

type JWT struct {
	AccessToken  string
	RefreshToken string
}

type RefreshToken struct {
	ID        uint64    `gorm:"column:id;type:bigserial;primaryKey;autoIncrement"`
	UserID    string    `gorm:"column:user_id;type:uuid;not null"`
	TokenHash string    `gorm:"column:token_hash;type:varchar(64);not null;uniqueIndex"`
	ExpiresAt time.Time `gorm:"column:expires_at;type:timestamptz;not null"`
	CreatedAt time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
}

func (RefreshToken) TableName() string {
	return "refresh_tokens"
}
