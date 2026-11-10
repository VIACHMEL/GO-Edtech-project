package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// LinkTeacherStudent establishes or restores a relationship.
// Authorization of the caller is a separate service-layer responsibility.
func (db *DB) LinkTeacherStudent(ctx context.Context, teacherID, studentID int64) error {
	const query = `
		INSERT INTO teacher_students (teacher_id, student_id, status)
		SELECT teacher.id, student.id, 'active'
		FROM users AS teacher
		CROSS JOIN users AS student
		WHERE teacher.id = $1 AND teacher.role = 'teacher' AND teacher.status = 'active'
		  AND student.id = $2 AND student.role = 'student' AND student.status = 'active'
		ON CONFLICT (teacher_id, student_id)
		DO UPDATE SET status = 'active', joined_at = NOW()
		RETURNING teacher_id`
	var id int64
	err := db.Pool.QueryRow(ctx, query, teacherID, studentID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: active teacher and student are required", ErrValidation)
	}
	if err != nil {
		return userDBError("link teacher student", err)
	}
	return nil
}

// UnlinkTeacherStudent revokes the link and removes the student from this
// teacher's groups in ONE transaction. Re-linking does not restore old groups.
func (db *DB) UnlinkTeacherStudent(ctx context.Context, teacherID, studentID int64) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return userDBError("begin unlink transaction", err)
	}
	defer tx.Rollback(ctx)
	const update = `UPDATE teacher_students SET status = 'removed'
		WHERE teacher_id = $1 AND student_id = $2 AND status = 'active'`
	tag, err := tx.Exec(ctx, update, teacherID, studentID)
	if err != nil {
		return userDBError("unlink teacher student", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	const removeMembership = `DELETE FROM group_members AS gm
		USING groups AS g
		WHERE gm.group_id = g.id AND g.teacher_id = $1 AND gm.student_id = $2`
	if _, err = tx.Exec(ctx, removeMembership, teacherID, studentID); err != nil {
		return userDBError("remove former group memberships", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return userDBError("commit unlink transaction", err)
	}
	return nil
}

// ListTeacherStudents reads student profiles via JOIN with the relationship table.
func (db *DB) ListTeacherStudents(ctx context.Context, teacherID int64) ([]User, error) {
	const query = `
		SELECT u.id, u.email, u.name, u.role, u.grade, u.status, u.created_at, u.updated_at
		FROM teacher_students AS ts
		JOIN users AS u ON u.id = ts.student_id
		WHERE ts.teacher_id = $1 AND ts.status = 'active' AND u.status = 'active'
		ORDER BY u.name, u.id`
	rows, err := db.Pool.Query(ctx, query, teacherID)
	if err != nil {
		return nil, userDBError("list teacher students", err)
	}
	defer rows.Close()
	users := make([]User, 0)
	for rows.Next() {
		u, err := scanPublicUser(rows)
		if err != nil {
			return nil, userDBError("scan teacher students", err)
		}
		users = append(users, *u)
	}
	if err := rows.Err(); err != nil {
		return nil, userDBError("iterate teacher students", err)
	}
	return users, nil
}

// ListStudentTeachers is a reverse lookup.
func (db *DB) ListStudentTeachers(ctx context.Context, studentID int64) ([]User, error) {
	const query = `
		SELECT u.id, u.email, u.name, u.role, u.grade, u.status, u.created_at, u.updated_at
		FROM teacher_students AS ts
		JOIN users AS u ON u.id = ts.teacher_id
		WHERE ts.student_id = $1 AND ts.status = 'active' AND u.status = 'active'
		ORDER BY u.name, u.id`
	rows, err := db.Pool.Query(ctx, query, studentID)
	if err != nil {
		return nil, userDBError("list student teachers", err)
	}
	defer rows.Close()
	users := make([]User, 0)
	for rows.Next() {
		u, err := scanPublicUser(rows)
		if err != nil {
			return nil, userDBError("scan student teachers", err)
		}
		users = append(users, *u)
	}
	if err := rows.Err(); err != nil {
		return nil, userDBError("iterate student teachers", err)
	}
	return users, nil
}
