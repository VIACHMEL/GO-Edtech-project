package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const assignmentColumns = "id,worksheet_id,student_id,assigned_by,group_id,distribution_id::text,due_at,status,assigned_at,max_attempts"
const attemptColumns = "id,assignment_id,worksheet_id,attempt_no,status,started_at,submitted_at,result_published_at,is_late"

func activeLearningUser(ctx context.Context, tx pgx.Tx, id int64, role string) error {
	var actual string
	err := tx.QueryRow(ctx, "SELECT role FROM users WHERE id=$1 AND status='active' AND role IN ('teacher','student') FOR SHARE", id).Scan(&actual)
	if err != nil {
		return worksheetError(err)
	}
	if role != "" && actual != role {
		return ErrNotFound
	}
	return nil
}

func (db *DB) learningTx(ctx context.Context, actor int64, role string, action func(pgx.Tx) error) error {
	if actor <= 0 {
		return ErrValidation
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = activeLearningUser(ctx, tx, actor, role); err != nil {
		return err
	}
	if err = action(tx); err != nil {
		return worksheetError(err)
	}
	return worksheetError(tx.Commit(ctx))
}

func scanAssignment(row pgx.Row) (*Assignment, error) {
	a := new(Assignment)
	err := row.Scan(&a.ID, &a.WorksheetID, &a.StudentID, &a.AssignedBy, &a.GroupID, &a.DistributionID, &a.DueAt, &a.Status, &a.AssignedAt, &a.MaxAttempts)
	if err != nil {
		return nil, worksheetError(err)
	}
	return a, nil
}

// assignmentForActor must be called after verifying the actor in the same transaction.
func assignmentForActor(ctx context.Context, tx pgx.Tx, actor, id int64) (*Assignment, error) {
	a, err := scanAssignment(tx.QueryRow(ctx, "SELECT "+assignmentColumns+` FROM assignments a
 WHERE a.id=$1 AND (a.student_id=$2 OR (a.assigned_by=$2 AND EXISTS(
 SELECT 1 FROM teacher_students ts WHERE ts.teacher_id=$2 AND ts.student_id=a.student_id AND ts.status='active')))
 FOR UPDATE`, id, actor))
	if err != nil {
		return nil, err
	}
	if a.AssignedBy == actor {
		var linked int
		if err := tx.QueryRow(ctx, "SELECT 1 FROM teacher_students WHERE teacher_id=$1 AND student_id=$2 AND status='active' FOR SHARE", actor, a.StudentID).Scan(&linked); err != nil {
			return nil, worksheetError(err)
		}
	}
	return a, nil
}

func runnableAssignment(ctx context.Context, tx pgx.Tx, a *Assignment) error {
	var allowed bool
	err := tx.QueryRow(ctx, `SELECT a.status='active'
 AND EXISTS(SELECT 1 FROM teacher_students ts JOIN users t ON t.id=ts.teacher_id
 WHERE ts.teacher_id=a.assigned_by AND ts.student_id=a.student_id AND ts.status='active'
 AND t.status='active' AND t.role='teacher') FROM assignments a WHERE a.id=$1`, a.ID).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrWorkClosed
	}
	var linked int
	if err := tx.QueryRow(ctx, "SELECT 1 FROM teacher_students ts JOIN users t ON t.id=ts.teacher_id WHERE ts.teacher_id=$1 AND ts.student_id=$2 AND ts.status='active' AND t.role='teacher' AND t.status='active' FOR SHARE OF ts,t", a.AssignedBy, a.StudentID).Scan(&linked); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrWorkClosed
		}
		return err
	}
	return nil
}

func sameDue(a, b *time.Time) bool {
	return a == nil && b == nil || a != nil && b != nil && a.Equal(*b)
}
func sameGroup(a, b *int64) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }

// AssignWorksheet uses a caller-supplied UUID as the stable key for one distribution.
func (db *DB) AssignWorksheet(ctx context.Context, teacher, worksheet, student int64, distribution string, due *time.Time) (*Assignment, error) {
	if student <= 0 {
		return nil, ErrValidation
	}
	result, err := db.distributeWorksheet(ctx, teacher, worksheet, &student, nil, distribution, due)
	if err != nil {
		return nil, err
	}
	return result[0], nil
}

// AssignWorksheetToGroup snapshots eligible members; retries with the same UUID reuse that snapshot.
func (db *DB) AssignWorksheetToGroup(ctx context.Context, teacher, worksheet, group int64, distribution string, due *time.Time) ([]*Assignment, error) {
	if group <= 0 {
		return nil, ErrValidation
	}
	return db.distributeWorksheet(ctx, teacher, worksheet, nil, &group, distribution, due)
}

func (db *DB) distributeWorksheet(ctx context.Context, teacher, worksheet int64, student, group *int64, distribution string, due *time.Time) ([]*Assignment, error) {
	var uuid pgtype.UUID
	distribution = strings.ToLower(strings.TrimSpace(distribution))
	if err := uuid.Scan(distribution); err != nil || !uuid.Valid {
		return nil, ErrValidation
	}
	if due != nil {
		d := due.UTC().Truncate(time.Microsecond)
		due = &d
	}
	result := make([]*Assignment, 0)
	err := db.mutateWorksheet(ctx, teacher, worksheet, false, func(tx pgx.Tx, w *Worksheet) error {
		// Serialize retries even if the same UUID is mistakenly used with another worksheet.
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", distribution); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, "SELECT "+assignmentColumns+" FROM assignments WHERE distribution_id=$1 ORDER BY id", distribution)
		if err != nil {
			return err
		}
		for rows.Next() {
			a, err := scanAssignment(rows)
			if err != nil {
				rows.Close()
				return err
			}
			result = append(result, a)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if len(result) > 0 {
			for _, a := range result {
				if a.AssignedBy != teacher || a.WorksheetID != worksheet || !sameGroup(a.GroupID, group) || !sameDue(a.DueAt, due) || student != nil && (len(result) != 1 || a.StudentID != *student) {
					return ErrConflict
				}
			}
			return nil
		}
		if w.Status != "published" {
			return ErrWorksheetLocked
		}
		students := []int64{}
		if group != nil {
			var g int64
			if err := tx.QueryRow(ctx, "SELECT id FROM groups WHERE id=$1 AND teacher_id=$2 AND status='active' FOR SHARE", *group, teacher).Scan(&g); err != nil {
				return worksheetError(err)
			}
			rows, err := tx.Query(ctx, `SELECT s.id FROM group_members gm JOIN users s ON s.id=gm.student_id
    JOIN teacher_students ts ON ts.teacher_id=$2 AND ts.student_id=s.id AND ts.status='active'
    WHERE gm.group_id=$1 AND s.role='student' AND s.status='active' ORDER BY s.id FOR SHARE OF s,ts,gm`, *group, teacher)
			if err != nil {
				return err
			}
			for rows.Next() {
				var id int64
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					return err
				}
				students = append(students, id)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return err
			}
			if len(students) == 0 {
				return fmt.Errorf("%w: no eligible group members", ErrValidation)
			}
		} else {
			var id int64
			err := tx.QueryRow(ctx, `SELECT s.id FROM users s JOIN teacher_students ts ON ts.student_id=s.id
    WHERE s.id=$1 AND s.role='student' AND s.status='active' AND ts.teacher_id=$2 AND ts.status='active' FOR SHARE OF s,ts`, *student, teacher).Scan(&id)
			if err != nil {
				return worksheetError(err)
			}
			students = append(students, id)
		}
		for _, id := range students {
			a, err := scanAssignment(tx.QueryRow(ctx, "INSERT INTO assignments(worksheet_id,student_id,assigned_by,group_id,distribution_id,due_at,max_attempts) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING "+assignmentColumns, worksheet, id, teacher, group, distribution, due, w.MaxAttempts))
			if err != nil {
				return err
			}
			result = append(result, a)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (db *DB) GetAssignment(ctx context.Context, actor, id int64) (*Assignment, error) {
	var result *Assignment
	err := db.learningTx(ctx, actor, "", func(tx pgx.Tx) error { var err error; result, err = assignmentForActor(ctx, tx, actor, id); return err })
	return result, err
}

func (db *DB) ListStudentAssignments(ctx context.Context, student, before int64, limit int) ([]*Assignment, error) {
	return db.listAssignments(ctx, student, "student", before, limit)
}
func (db *DB) ListTeacherAssignments(ctx context.Context, teacher, before int64, limit int) ([]*Assignment, error) {
	return db.listAssignments(ctx, teacher, "teacher", before, limit)
}
func (db *DB) listAssignments(ctx context.Context, actor int64, role string, before int64, limit int) ([]*Assignment, error) {
	if before < 0 {
		return nil, ErrValidation
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	result := make([]*Assignment, 0)
	err := db.learningTx(ctx, actor, role, func(tx pgx.Tx) error {
		filter := "student_id=$1"
		if role == "teacher" {
			filter = "assigned_by=$1 AND EXISTS(SELECT 1 FROM teacher_students ts WHERE ts.teacher_id=$1 AND ts.student_id=assignments.student_id AND ts.status='active')"
		}
		rows, err := tx.Query(ctx, "SELECT "+assignmentColumns+" FROM assignments WHERE "+filter+" AND ($2::bigint=0 OR id<$2) ORDER BY id DESC LIMIT $3", actor, before, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			a, err := scanAssignment(rows)
			if err != nil {
				return err
			}
			result = append(result, a)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (db *DB) SetAssignmentStatus(ctx context.Context, teacher, id int64, status string) error {
	if status != "active" && status != "closed" && status != "cancelled" {
		return ErrValidation
	}
	return db.learningTx(ctx, teacher, "teacher", func(tx pgx.Tx) error {
		a, err := assignmentForActor(ctx, tx, teacher, id)
		if err != nil {
			return err
		}
		if a.AssignedBy != teacher {
			return ErrNotFound
		}
		_, err = tx.Exec(ctx, "UPDATE assignments SET status=$2 WHERE id=$1", id, status)
		return err
	})
}

func (db *DB) CloseAssignment(ctx context.Context, teacher, id int64) error {
	return db.SetAssignmentStatus(ctx, teacher, id, "closed")
}

func (db *DB) UpdateAssignmentDeadline(ctx context.Context, teacher, id int64, due *time.Time) error {
	return db.learningTx(ctx, teacher, "teacher", func(tx pgx.Tx) error {
		a, err := assignmentForActor(ctx, tx, teacher, id)
		if err != nil {
			return err
		}
		if a.AssignedBy != teacher {
			return ErrNotFound
		}
		_, err = tx.Exec(ctx, "UPDATE assignments SET due_at=$2 WHERE id=$1", id, due)
		return err
	})
}

// scanAttempt deliberately contains no answers or grading internals.
func scanAttempt(row pgx.Row) (*Attempt, error) {
	a := new(Attempt)
	err := row.Scan(&a.ID, &a.AssignmentID, &a.WorksheetID, &a.AttemptNo, &a.Status, &a.StartedAt, &a.SubmittedAt, &a.ResultPublishedAt, &a.IsLate)
	if err != nil {
		return nil, worksheetError(err)
	}
	return a, nil
}

func attemptForActor(ctx context.Context, tx pgx.Tx, actor, id int64) (*Attempt, *Assignment, error) {
	var assignmentID int64
	if err := tx.QueryRow(ctx, "SELECT assignment_id FROM attempts WHERE id=$1", id).Scan(&assignmentID); err != nil {
		return nil, nil, worksheetError(err)
	}
	a, err := assignmentForActor(ctx, tx, actor, assignmentID)
	if err != nil {
		return nil, nil, err
	}
	attempt, err := scanAttempt(tx.QueryRow(ctx, "SELECT "+attemptColumns+" FROM attempts WHERE id=$1 FOR UPDATE", id))
	return attempt, a, err
}

func createAttempt(ctx context.Context, tx pgx.Tx, a *Assignment) (*Attempt, error) {
	var used int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM attempts WHERE assignment_id=$1", a.ID).Scan(&used); err != nil {
		return nil, err
	}
	if used >= a.MaxAttempts {
		return nil, ErrAttemptLimit
	}
	attempt, err := scanAttempt(tx.QueryRow(ctx, "INSERT INTO attempts(assignment_id,worksheet_id,attempt_no) SELECT $1,$2,COALESCE(MAX(attempt_no),0)+1 FROM attempts WHERE assignment_id=$1 RETURNING "+attemptColumns, a.ID, a.WorksheetID))
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, "INSERT INTO attempt_answers(attempt_id,worksheet_id,worksheet_item_id) SELECT $1,$2,id FROM worksheet_items WHERE worksheet_id=$2", attempt.ID, a.WorksheetID)
	if err != nil {
		return nil, err
	}
	return attempt, nil
}

func (db *DB) StartOrResumeAttempt(ctx context.Context, student, assignmentID int64) (*Attempt, error) {
	var result *Attempt
	err := db.learningTx(ctx, student, "student", func(tx pgx.Tx) error {
		a, err := assignmentForActor(ctx, tx, student, assignmentID)
		if err != nil {
			return err
		}
		if a.StudentID != student {
			return ErrNotFound
		}
		if err := runnableAssignment(ctx, tx, a); err != nil {
			return err
		}
		result, err = scanAttempt(tx.QueryRow(ctx, "SELECT "+attemptColumns+" FROM attempts WHERE assignment_id=$1 ORDER BY attempt_no DESC LIMIT 1", a.ID))
		if errors.Is(err, ErrNotFound) {
			result, err = createAttempt(ctx, tx, a)
			return err
		}
		if err != nil {
			return err
		}
		if result.Status != "draft" {
			result, err = createAttempt(ctx, tx, a)
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ReopenAttempt creates or resumes a draft within the existing attempt limit.
func (db *DB) ReopenAttempt(ctx context.Context, teacher, assignmentID int64) (*Attempt, error) {
	var result *Attempt
	err := db.learningTx(ctx, teacher, "teacher", func(tx pgx.Tx) error {
		a, err := assignmentForActor(ctx, tx, teacher, assignmentID)
		if err != nil {
			return err
		}
		if a.AssignedBy != teacher {
			return ErrNotFound
		}
		if err := runnableAssignment(ctx, tx, a); err != nil {
			return err
		}
		result, err = scanAttempt(tx.QueryRow(ctx, "SELECT "+attemptColumns+" FROM attempts WHERE assignment_id=$1 AND status='draft'", a.ID))
		if err == nil {
			return nil
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}
		result, err = createAttempt(ctx, tx, a)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (db *DB) GetAttempt(ctx context.Context, actor, id int64) (*Attempt, error) {
	var result *Attempt
	err := db.learningTx(ctx, actor, "", func(tx pgx.Tx) error { var err error; result, _, err = attemptForActor(ctx, tx, actor, id); return err })
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (db *DB) ListAssignmentAttempts(ctx context.Context, actor, assignmentID int64) ([]*Attempt, error) {
	result := make([]*Attempt, 0)
	err := db.learningTx(ctx, actor, "", func(tx pgx.Tx) error {
		if _, err := assignmentForActor(ctx, tx, actor, assignmentID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, "SELECT "+attemptColumns+" FROM attempts WHERE assignment_id=$1 ORDER BY attempt_no", assignmentID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			a, err := scanAttempt(rows)
			if err != nil {
				return err
			}
			result = append(result, a)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
