package db

import (
	"context"
	"fmt"
)

type StudentGroupView struct {
	ID   int64
	Name string
}

type TeacherStudentView struct {
	ID     int64
	Name   string
	Email  string
	Grade  *int16
	Status string
	Groups []StudentGroupView
}

func (db *DB) ListTeacherStudentViews(
	ctx context.Context,
	teacherID int64,
) ([]TeacherStudentView, error) {

	if teacherID <= 0 {
		return nil, fmt.Errorf(
			"%w: teacher ID must be positive", ErrValidation,
		)
	}

	const query = `
        SELECT
            u.id,
            u.name,
            u.email,
            u.grade,
            u.status,
            linked.group_id,
            linked.group_name
        FROM teacher_students AS ts
        JOIN users AS teacher
            ON teacher.id = ts.teacher_id
            AND teacher.role = 'teacher'
            AND teacher.status = 'active'
        JOIN users AS u
            ON u.id = ts.student_id
        LEFT JOIN (
            SELECT
                gm.student_id,
                g.id AS group_id,
                g.name AS group_name
            FROM group_members AS gm
            JOIN groups AS g ON g.id = gm.group_id
            WHERE g.teacher_id = $1
              AND g.status = 'active'
        ) AS linked
            ON linked.student_id = u.id
        WHERE ts.teacher_id = $1
          AND ts.status = 'active'
          AND u.role = 'student'
          AND u.status = 'active'
        ORDER BY u.name, u.id, linked.group_name
    `

	rows, err := db.Pool.Query(ctx, query, teacherID)
	if err != nil {
		return nil, userDBError("list student views", err)
	}
	defer rows.Close()

	result := make([]TeacherStudentView, 0)
	positions := make(map[int64]int)

	for rows.Next() {
		var id int64
		var name, email, status string
		var grade *int16
		var groupID *int64
		var groupName *string

		err := rows.Scan(
			&id, &name, &email, &grade, &status,
			&groupID, &groupName,
		)
		if err != nil {
			return nil, userDBError("scan student view", err)
		}

		index, exists := positions[id]

		if !exists {
			index = len(result)
			positions[id] = index

			result = append(result, TeacherStudentView{
				ID:     id,
				Name:   name,
				Email:  email,
				Grade:  grade,
				Status: status,
				Groups: make([]StudentGroupView, 0),
			})
		}

		if groupID != nil && groupName != nil {
			result[index].Groups = append(
				result[index].Groups,
				StudentGroupView{
					ID:   *groupID,
					Name: *groupName,
				},
			)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, userDBError("iterate student views", err)
	}

	return result, nil
}

// TeacherGroupMemberView is one active student in a teacher's group.
// Membership is filtered through the current active teacher-student link.
type TeacherGroupMemberView struct {
	ID    int64
	Name  string
	Email string
}

// TeacherGroupView contains a real group (active or archived) and its currently
// active, linked students. No lesson, homework, or statistics data is fabricated.
type TeacherGroupView struct {
	ID      int64
	Name    string
	Subject *string
	Status  string
	Members []TeacherGroupMemberView
}

// ListTeacherGroupViews fetches both active and archived groups using ONE query.
// The caller must authenticate the teacher and authorize access to these data.
func (db *DB) ListTeacherGroupViews(ctx context.Context, teacherID int64) ([]TeacherGroupView, error) {
	if teacherID <= 0 {
		return nil, fmt.Errorf("%w: teacher ID must be positive", ErrValidation)
	}

	const query = `
        SELECT g.id, g.name, g.subject, g.status,
               u.id, u.name, u.email
        FROM groups AS g
        JOIN users AS teacher
             ON teacher.id = g.teacher_id
            AND teacher.role = 'teacher'
            AND teacher.status = 'active'
        LEFT JOIN group_members AS gm
             ON gm.group_id = g.id
        LEFT JOIN teacher_students AS ts
             ON ts.teacher_id = g.teacher_id
            AND ts.student_id = gm.student_id
            AND ts.status = 'active'
        LEFT JOIN users AS u
             ON u.id = gm.student_id
            AND u.role = 'student'
            AND u.status = 'active'
            AND ts.student_id IS NOT NULL
        WHERE g.teacher_id = $1
        ORDER BY g.name, g.id, u.name, u.id
    `

	rows, err := db.Pool.Query(ctx, query, teacherID)
	if err != nil {
		return nil, userDBError("list teacher group views", err)
	}
	defer rows.Close()

	result := make([]TeacherGroupView, 0)
	positions := make(map[int64]int)
	for rows.Next() {
		var groupID int64
		var name, status string
		var subject *string
		var studentID *int64
		var studentName, studentEmail *string
		if err := rows.Scan(&groupID, &name, &subject, &status,
			&studentID, &studentName, &studentEmail); err != nil {
			return nil, userDBError("scan teacher group view", err)
		}

		index, exists := positions[groupID]
		if !exists {
			index = len(result)
			positions[groupID] = index
			result = append(result, TeacherGroupView{
				ID: groupID, Name: name, Subject: subject, Status: status,
				Members: make([]TeacherGroupMemberView, 0),
			})
		}
		if studentID != nil && studentName != nil && studentEmail != nil {
			result[index].Members = append(result[index].Members, TeacherGroupMemberView{
				ID: *studentID, Name: *studentName, Email: *studentEmail,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, userDBError("iterate teacher group views", err)
	}
	return result, nil
}
