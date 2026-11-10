package db

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const userPublicColumns = `id, email, name, role, grade, status, created_at, updated_at`

func scanPublicUser(row pgx.Row) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.Grade,
		&u.Status, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func userDBError(action string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return fmt.Errorf("%w: %s", ErrConflict, action)
		case "23503", "23514", "23502": // foreign_key, check, not_null
			return fmt.Errorf("%w: %s", ErrValidation, action)
		}
	}
	return fmt.Errorf("%s: %w", action, err)
}

func normalizeUserEmail(value string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(value))
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email {
		return "", fmt.Errorf("%w: invalid email", ErrValidation)
	}
	return email, nil
}

func validateNewUser(input NewUser) (NewUser, error) {
	var err error
	input.Email, err = normalizeUserEmail(input.Email)
	if err != nil {
		return input, err
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || strings.TrimSpace(input.PasswordHash) == "" {
		return input, fmt.Errorf("%w: name and password hash are required", ErrValidation)
	}
	if input.Role != "student" && input.Role != "teacher" {
		return input, fmt.Errorf("%w: role must be student or teacher", ErrValidation)
	}
	if input.Role != "student" && input.Grade != nil {
		return input, fmt.Errorf("%w: grade is only for students", ErrValidation)
	}
	if input.Grade != nil && (*input.Grade < 1 || *input.Grade > 12) {
		return input, fmt.Errorf("%w: grade must be 1..12", ErrValidation)
	}
	return input, nil
}

// CreateUser inserts a new account and returns its public profile.
func (db *DB) CreateUser(ctx context.Context, input NewUser) (*User, error) {
	input, err := validateNewUser(input)
	if err != nil {
		return nil, err
	}
	query := `INSERT INTO users (email, password_hash, name, role, grade)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING ` + userPublicColumns
	user, err := scanPublicUser(db.Pool.QueryRow(ctx, query,
		input.Email, input.PasswordHash, input.Name, input.Role, input.Grade))
	if err != nil {
		return nil, userDBError("create user", err)
	}
	return user, nil
}

// GetUser never returns password_hash.
func (db *DB) GetUser(ctx context.Context, id int64) (*User, error) {
	query := `SELECT ` + userPublicColumns + ` FROM users WHERE id = $1`
	user, err := scanPublicUser(db.Pool.QueryRow(ctx, query, id))
	if err != nil {
		return nil, userDBError("get user", err)
	}
	return user, nil
}

// GetUserByEmail is only for the authentication service.
func (db *DB) GetUserByEmail(ctx context.Context, value string) (*UserCredentials, error) {
	email, err := normalizeUserEmail(value)
	if err != nil {
		return nil, err
	}
	const query = `SELECT id, email, password_hash, role, status
		FROM users WHERE lower(email) = $1`
	var credentials UserCredentials
	err = db.Pool.QueryRow(ctx, query, email).Scan(
		&credentials.ID, &credentials.Email, &credentials.PasswordHash,
		&credentials.Role, &credentials.Status)
	if err != nil {
		return nil, userDBError("get user by email", err)
	}
	return &credentials, nil
}

// UpdateUserProfile changes only safe profile fields, not roles or password hashes.
func (db *DB) UpdateUserProfile(ctx context.Context, id int64, name string, grade *int16) (*User, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrValidation)
	}
	if grade != nil && (*grade < 1 || *grade > 12) {
		return nil, fmt.Errorf("%w: grade must be 1..12", ErrValidation)
	}
	query := `UPDATE users SET name = $2, grade = $3, updated_at = NOW()
		WHERE id = $1 AND (role = 'student' OR $3::smallint IS NULL)
		RETURNING ` + userPublicColumns
	user, err := scanPublicUser(db.Pool.QueryRow(ctx, query, id, name, grade))
	if err != nil {
		return nil, userDBError("update user", err)
	}
	return user, nil
}

// DisableUser is soft deletion; foreign keys remain valid.
func (db *DB) DisableUser(ctx context.Context, id int64) error {
	const query = `UPDATE users SET status = 'disabled', updated_at = NOW() WHERE id = $1`
	tag, err := db.Pool.Exec(ctx, query, id)
	if err != nil {
		return userDBError("disable user", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
