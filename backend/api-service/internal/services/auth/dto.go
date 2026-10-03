package auth

import (
	"time"

	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/model"
)

type RegisterRequest struct {
	Email    string `json:"email" binding:"required,email,max=254"`
	Password string `json:"password" binding:"required,min=8"`
	Name     string `json:"name" binding:"required,max=100"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email,max=254"`
	Password string `json:"password" binding:"required"`
}

type UserResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}
type JWTResponse struct {
	AccessToken string `json:"accessToken"`
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type AuthResponse struct {
	AccessToken string       `json:"access_token"`
	TokenType   string       `json:"token_type"`
	ExpiresIn   int          `json:"expires_in"`
	User        UserResponse `json:"user"`
}

type AuthResult struct {
	AccessToken      string
	AccessExpiresIn  time.Duration
	RefreshToken     string
	RefreshExpiresAt time.Time
	User             *model.User
}

func ToUserResponse(u *model.User) UserResponse {
	return UserResponse{ID: u.ID, Email: u.Email, Name: u.Name}
}

func ToJWTResponse(t *model.JWT) JWTResponse {
	return JWTResponse{AccessToken: t.AccessToken}
}

func ToAuthResponse(r *AuthResult) AuthResponse {
	return AuthResponse{
		AccessToken: r.AccessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int(r.AccessExpiresIn.Seconds()),
		User:        ToUserResponse(r.User),
	}
}
