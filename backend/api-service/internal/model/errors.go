package model

import "errors"

var (
	ErrEmailTaken         = errors.New("user: email already registered")
	ErrUserNotFound       = errors.New("user: not found")
	ErrInvalidCredentials = errors.New("user: invalid credentials")
	ErrInvalidToken       = errors.New("user: invalid or expired token")
	ErrWeakPassword       = errors.New("user: password must be 8-72 bytes long")
	ErrInvalidInput       = errors.New("user: invalid input")
	ErrInvalidAccessToken = errors.New("user: invalid access token")
)
