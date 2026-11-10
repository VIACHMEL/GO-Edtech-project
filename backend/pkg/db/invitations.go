package db

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const invitationPublicColumns = `id, teacher_id, group_id, invitee_email,
	status, expires_at, accepted_by, accepted_at, created_at`

func scanInvitation(row pgx.Row) (*Invitation, error) {
	var i Invitation
	err := row.Scan(&i.ID, &i.TeacherID, &i.GroupID, &i.InviteeEmail,
		&i.Status, &i.ExpiresAt, &i.AcceptedBy, &i.AcceptedAt, &i.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &i, nil
}

// validateTokenHash ensures that only a SHA-256 hex digest is stored/searched.
// The server generates a cryptographically random raw token and hashes it.
func validateTokenHash(value string) (string, error) {
	if len(value) != 64 || strings.ToLower(value) != value {
		return "", fmt.Errorf("%w: token hash must be 64 lowercase hex chars", ErrValidation)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return "", fmt.Errorf("%w: invalid token hash", ErrValidation)
	}
	return value, nil
}

func validateNewInvitation(input NewInvitation) (NewInvitation, error) {
	if input.TeacherID <= 0 || (input.GroupID != nil && *input.GroupID <= 0) {
		return input, fmt.Errorf("%w: invalid teacher or group id", ErrValidation)
	}
	var err error
	input.TokenHash, err = validateTokenHash(input.TokenHash)
	if err != nil {
		return input, err
	}
	if !input.ExpiresAt.After(time.Now()) {
		return input, fmt.Errorf("%w: expiration must be in the future", ErrValidation)
	}
	if input.InviteeEmail != nil {
		email, err := normalizeUserEmail(*input.InviteeEmail)
		if err != nil {
			return input, err
		}
		input.InviteeEmail = &email
	}
	return input, nil
}

// CreateInvitation creates an invitation linked to an existing active teacher.
// Optional group must be active and belong to that teacher.
func (db *DB) CreateInvitation(ctx context.Context, input NewInvitation) (*Invitation, error) {
	input, err := validateNewInvitation(input)
	if err != nil {
		return nil, err
	}
	const query = `INSERT INTO invitations (teacher_id, group_id, invitee_email, token_hash, expires_at)
		SELECT teacher.id, $2::bigint, $3::text, $4::text, $5::timestamptz
		FROM users AS teacher
		WHERE teacher.id = $1 AND teacher.role = 'teacher' AND teacher.status = 'active'
		  AND ($2::bigint IS NULL OR EXISTS (
		      SELECT 1 FROM groups g WHERE g.id = $2 AND g.teacher_id = teacher.id AND g.status = 'active'))
		RETURNING ` + invitationPublicColumns
	inv, err := scanInvitation(db.Pool.QueryRow(ctx, query,
		input.TeacherID, input.GroupID, input.InviteeEmail, input.TokenHash, input.ExpiresAt))
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("%w: teacher or group is invalid", ErrValidation)
	}
	if err != nil {
		return nil, userDBError("create invitation", err)
	}
	return inv, nil
}

// GetInvitationByTokenHash looks up the invitation without revealing its hash.
func (db *DB) GetInvitationByTokenHash(ctx context.Context, tokenHash string) (*Invitation, error) {
	var err error
	tokenHash, err = validateTokenHash(tokenHash)
	if err != nil {
		return nil, err
	}
	const query = `SELECT ` + invitationPublicColumns + ` FROM invitations WHERE token_hash = $1`
	inv, err := scanInvitation(db.Pool.QueryRow(ctx, query, tokenHash))
	if err != nil {
		return nil, userDBError("get invitation", err)
	}
	return inv, nil
}

// RevokeInvitation invalidates a still-pending invitation.
func (db *DB) RevokeInvitation(ctx context.Context, invitationID, teacherID int64) error {
	const query = `UPDATE invitations SET status = 'revoked'
		WHERE id = $1 AND teacher_id = $2 AND status = 'pending'`
	tag, err := db.Pool.Exec(ctx, query, invitationID, teacherID)
	if err != nil {
		return userDBError("revoke invitation", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AcceptInvitation atomically links the student to the teacher, optionally
// adds the student to a group, and marks the one-use invitation accepted.
// The authenticated server must supply the actual logged-in studentID.
func (db *DB) AcceptInvitation(ctx context.Context, tokenHash string, studentID int64) error {
	var err error
	tokenHash, err = validateTokenHash(tokenHash)
	if err != nil {
		return err
	}
	if studentID <= 0 {
		return fmt.Errorf("%w: student id must be positive", ErrValidation)
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return userDBError("begin invitation transaction", err)
	}
	defer tx.Rollback(ctx) // safe after successful Commit

	const lockQuery = `SELECT ` + invitationPublicColumns + `
		FROM invitations WHERE token_hash = $1 FOR UPDATE`
	inv, err := scanInvitation(tx.QueryRow(ctx, lockQuery, tokenHash))
	if err != nil {
		return userDBError("lock invitation", err)
	}
	if inv.Status != "pending" {
		return fmt.Errorf("%w: invitation is already used or revoked", ErrConflict)
	}
	if !inv.ExpiresAt.After(time.Now()) {
		return fmt.Errorf("%w: invitation has expired", ErrValidation)
	}

	var studentEmail string
	err = tx.QueryRow(ctx, `SELECT email FROM users
		WHERE id = $1 AND role = 'student' AND status = 'active'`, studentID).Scan(&studentEmail)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("%w: active student required", ErrValidation)
	}
	if err != nil {
		return userDBError("check student", err)
	}
	if inv.InviteeEmail != nil && !strings.EqualFold(*inv.InviteeEmail, studentEmail) {
		return fmt.Errorf("%w: invitation belongs to another email", ErrValidation)
	}

	var teacherAndGroupValid bool
	err = tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM users AS teacher
		WHERE teacher.id = $1 AND teacher.role = 'teacher' AND teacher.status = 'active'
		AND ($2::bigint IS NULL OR EXISTS (
		    SELECT 1 FROM groups g
		    WHERE g.id = $2 AND g.teacher_id = teacher.id AND g.status = 'active'))
	)`, inv.TeacherID, inv.GroupID).Scan(&teacherAndGroupValid)
	if err != nil {
		return userDBError("check teacher and group", err)
	}
	if !teacherAndGroupValid {
		return fmt.Errorf("%w: teacher or group is inactive", ErrValidation)
	}

	_, err = tx.Exec(ctx, `INSERT INTO teacher_students (teacher_id, student_id, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (teacher_id, student_id)
		DO UPDATE SET status = 'active', joined_at = NOW()`, inv.TeacherID, studentID)
	if err != nil {
		return userDBError("link invited student", err)
	}
	if inv.GroupID != nil {
		_, err = tx.Exec(ctx, `INSERT INTO group_members (group_id, student_id)
			VALUES ($1, $2) ON CONFLICT (group_id, student_id) DO NOTHING`, *inv.GroupID, studentID)
		if err != nil {
			return userDBError("join invited group", err)
		}
	}
	_, err = tx.Exec(ctx, `UPDATE invitations
		SET status = 'accepted', accepted_by = $2, accepted_at = NOW()
		WHERE id = $1`, inv.ID, studentID)
	if err != nil {
		return userDBError("mark invitation accepted", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return userDBError("commit invitation", err)
	}
	return nil
}
