package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

const groupColumns = "id, teacher_id, name, subject, status, created_at"

// scanGroup converts one PostgreSQL row into a Group struct.
func scanGroup(row pgx.Row) (*Group, error) {
	var group Group
	err := row.Scan(&group.ID, &group.TeacherID, &group.Name,
		&group.Subject, &group.Status, &group.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &group, nil
}

func validateGroupInput(name string, subject *string) (string, *string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil, fmt.Errorf("%w: group name is required", ErrValidation)
	}
	if subject != nil {
		trimmed := strings.TrimSpace(*subject)
		if trimmed == "" {
			subject = nil
		} else {
			subject = &trimmed
		}
	}
	return name, subject, nil
}

// CreateGroup creates a group only for an active teacher.
// Caller authorization is checked by the HTTP/service layer.
func (db *DB) CreateGroup(ctx context.Context, input NewGroup) (*Group, error) {
	name, subject, err := validateGroupInput(input.Name, input.Subject)
	if err != nil {
		return nil, err
	}
	if input.TeacherID <= 0 {
		return nil, fmt.Errorf("%w: teacher id must be positive", ErrValidation)
	}
	const query = `INSERT INTO groups (teacher_id, name, subject)
		SELECT id, $2, $3 FROM users
		WHERE id = $1 AND role = 'teacher' AND status = 'active'
		RETURNING ` + groupColumns
	group, err := scanGroup(db.Pool.QueryRow(ctx, query, input.TeacherID, name, subject))
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("%w: active teacher required", ErrValidation)
	}
	if err != nil {
		return nil, userDBError("create group", err)
	}
	return group, nil
}

func (db *DB) GetGroup(ctx context.Context, groupID int64) (*Group, error) {
	const query = `SELECT ` + groupColumns + ` FROM groups WHERE id = $1`
	group, err := scanGroup(db.Pool.QueryRow(ctx, query, groupID))
	if err != nil {
		return nil, userDBError("get group", err)
	}
	return group, nil
}

// UpdateGroup updates a group only if it belongs to the supplied teacher.
func (db *DB) UpdateGroup(ctx context.Context, groupID, teacherID int64, name string, subject *string) (*Group, error) {
	name, subject, err := validateGroupInput(name, subject)
	if err != nil {
		return nil, err
	}
	const query = `UPDATE groups SET name = $3, subject = $4
		WHERE id = $1 AND teacher_id = $2 AND status = 'active'
		RETURNING ` + groupColumns
	group, err := scanGroup(db.Pool.QueryRow(ctx, query, groupID, teacherID, name, subject))
	if err != nil {
		return nil, userDBError("update group", err)
	}
	return group, nil
}

func (db *DB) ListTeacherGroups(ctx context.Context, teacherID int64) ([]Group, error) {
	const query = `SELECT ` + groupColumns + ` FROM groups
		WHERE teacher_id = $1 AND status = 'active' ORDER BY id`
	rows, err := db.Pool.Query(ctx, query, teacherID)
	if err != nil {
		return nil, userDBError("list groups", err)
	}
	defer rows.Close()
	groups := make([]Group, 0)
	for rows.Next() {
		g, err := scanGroup(rows)
		if err != nil {
			return nil, userDBError("scan groups", err)
		}
		groups = append(groups, *g)
	}
	if err := rows.Err(); err != nil {
		return nil, userDBError("iterate groups", err)
	}
	return groups, nil
}

// ArchiveGroup preserves references and membership history.
func (db *DB) ArchiveGroup(ctx context.Context, groupID, teacherID int64) error {
	const query = `UPDATE groups SET status = 'archived'
		WHERE id = $1 AND teacher_id = $2 AND status = 'active'`
	tag, err := db.Pool.Exec(ctx, query, groupID, teacherID)
	if err != nil {
		return userDBError("archive group", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AddGroupMember requires an active group, student, and teacher-student link.
func (db *DB) AddGroupMember(ctx context.Context, groupID, studentID int64) error {
	const query = `INSERT INTO group_members (group_id, student_id)
		SELECT g.id, s.id
		FROM groups AS g
		JOIN users AS teacher ON teacher.id = g.teacher_id
		    AND teacher.role = 'teacher' AND teacher.status = 'active'
		JOIN teacher_students AS ts ON ts.teacher_id = g.teacher_id
		JOIN users AS s ON s.id = ts.student_id
		WHERE g.id = $1 AND g.status = 'active'
		  AND s.id = $2 AND s.role = 'student' AND s.status = 'active'
		  AND ts.status = 'active'
		ON CONFLICT (group_id, student_id)
		DO UPDATE SET joined_at = group_members.joined_at
		RETURNING group_id`
	var id int64
	err := db.Pool.QueryRow(ctx, query, groupID, studentID).Scan(&id)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("%w: active group and linked student are required", ErrValidation)
	}
	if err != nil {
		return userDBError("add group member", err)
	}
	return nil
}

func (db *DB) RemoveGroupMember(ctx context.Context, groupID, studentID int64) error {
	const query = `DELETE FROM group_members WHERE group_id = $1 AND student_id = $2`
	tag, err := db.Pool.Exec(ctx, query, groupID, studentID)
	if err != nil {
		return userDBError("remove group member", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (db *DB) ListGroupMembers(ctx context.Context, groupID int64) ([]User, error) {
	const query = `SELECT u.id, u.email, u.name, u.role, u.grade, u.status, u.created_at, u.updated_at
		FROM group_members AS gm
		JOIN groups AS g ON g.id = gm.group_id
		JOIN users AS teacher ON teacher.id = g.teacher_id
		    AND teacher.role = 'teacher' AND teacher.status = 'active'
		JOIN teacher_students AS ts ON ts.teacher_id = g.teacher_id
		    AND ts.student_id = gm.student_id AND ts.status = 'active'
		JOIN users AS u ON u.id = gm.student_id
		WHERE gm.group_id = $1 AND g.status = 'active' AND u.status = 'active' 
		ORDER BY u.name, u.id`
	rows, err := db.Pool.Query(ctx, query, groupID)
	if err != nil {
		return nil, userDBError("list group members", err)
	}
	defer rows.Close()
	members := make([]User, 0)
	for rows.Next() {
		u, err := scanPublicUser(rows)
		if err != nil {
			return nil, userDBError("scan group members", err)
		}
		members = append(members, *u)
	}
	if err := rows.Err(); err != nil {
		return nil, userDBError("iterate group members", err)
	}
	return members, nil
}
