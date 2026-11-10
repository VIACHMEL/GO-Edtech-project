package db

import (
	"errors"
	"time"
)

// ErrConflict means that a database uniqueness constraint was violated.
var ErrConflict = errors.New("db: conflict")

// User is safe for a public profile: it has no password hash.
type User struct {
	ID        int64
	Email     string
	Name      string
	Role      string
	Grade     *int16 // nil means NULL in PostgreSQL
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewUser is a storage input, NOT the public signup request.
// PasswordHash must be created by the authentication service.
type NewUser struct {
	Email        string
	PasswordHash string
	Name         string
	Role         string
	Grade        *int16
}

// UserCredentials contains a password hash: never serialize it to the client.
type UserCredentials struct {
	ID           int64
	Email        string
	PasswordHash string
	Role         string
	Status       string
}
