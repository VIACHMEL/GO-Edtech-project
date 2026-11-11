package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

func editableAttempt(ctx context.Context, tx pgx.Tx, actor, attemptID int64) (*Attempt, error) {
	attempt, assignment, err := attemptForActor(ctx, tx, actor, attemptID)
	if err != nil {
		return nil, err
	}
	if assignment.StudentID != actor {
		return nil, ErrNotFound
	}
	if attempt.Status != "draft" {
		return nil, ErrAttemptLocked
	}
	if err := runnableAssignment(ctx, tx, assignment); err != nil {
		return nil, err
	}
	return attempt, nil
}

// SaveAnswer uses optimistic revisions; clients must submit the last revision they received.
func (db *DB) SaveAnswer(ctx context.Context, student, attemptID int64, input AnswerInput) (*SavedAnswer, error) {
	input.Text = strings.TrimSpace(input.Text)
	data, err := json.Marshal(input.Data)
	if err != nil || len(data) > 1024*1024 || utf8.RuneCountInString(input.Text) > 10000 || input.ExpectedRevision < 0 || input.WorksheetItemID <= 0 {
		return nil, ErrValidation
	}
	result := &SavedAnswer{WorksheetItemID: input.WorksheetItemID}
	err = db.learningTx(ctx, student, "student", func(tx pgx.Tx) error {
		if _, err := editableAttempt(ctx, tx, student, attemptID); err != nil {
			return err
		}
		var revision int
		var currentText string
		var currentData map[string]any
		err := tx.QueryRow(ctx, "SELECT answer_revision,COALESCE(answer_text,''),answer_data FROM attempt_answers WHERE attempt_id=$1 AND worksheet_item_id=$2", attemptID, input.WorksheetItemID).Scan(&revision, &currentText, &currentData)
		if err != nil {
			return worksheetError(err)
		}
		if revision != input.ExpectedRevision {
			// A retry of the immediately preceding identical save returns the same revision.
			old, _ := json.Marshal(currentData)
			if revision == input.ExpectedRevision+1 && currentText == input.Text && string(old) == string(data) {
				result.AnswerRevision = revision
				return nil
			}
			return ErrRevisionConflict
		}
		if currentText == input.Text && string(data) == "null" && currentData == nil {
			result.AnswerRevision = revision
			return nil
		}
		err = tx.QueryRow(ctx, `UPDATE attempt_answers SET answer_text=NULLIF($3,''),answer_data=$4,
   answer_revision=answer_revision+1,is_correct=NULL,score=NULL,grade_status='pending',graded_revision=NULL,
   graded_by=NULL,teacher_comment=NULL,updated_at=now()
   WHERE attempt_id=$1 AND worksheet_item_id=$2 RETURNING answer_revision`,
			attemptID, input.WorksheetItemID, input.Text, input.Data).Scan(&result.AnswerRevision)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (db *DB) ClearAnswer(ctx context.Context, student, attemptID, itemID int64, revision int) (*SavedAnswer, error) {
	return db.SaveAnswer(ctx, student, attemptID, AnswerInput{WorksheetItemID: itemID, ExpectedRevision: revision})
}

func autoGradeSubmitted(ctx context.Context, tx pgx.Tx, attemptID int64) error {
	_, err := tx.Exec(ctx, `UPDATE attempt_answers a SET
 is_correct=(btrim(COALESCE(a.answer_text,''))<>'' AND btrim(a.answer_text)=btrim(i.answer_snapshot)),
 score=CASE WHEN btrim(COALESCE(a.answer_text,''))<>'' AND btrim(a.answer_text)=btrim(i.answer_snapshot) THEN i.max_score ELSE 0 END,
 grade_status='auto',graded_revision=a.answer_revision,graded_by=NULL,teacher_comment=NULL,updated_at=now()
 FROM worksheet_items i WHERE a.attempt_id=$1 AND i.id=a.worksheet_item_id AND a.grade_status<>'manual'
 AND ((i.task_type_snapshot='short_answer' AND btrim(COALESCE(i.answer_snapshot,''))<>'')
 OR (btrim(COALESCE(a.answer_text,''))='' AND a.answer_data IS NULL))`, attemptID)
	return err
}

// SubmitAttempt saves no additional input: the student's latest draft is submitted atomically.
func (db *DB) SubmitAttempt(ctx context.Context, student, attemptID int64) (*Attempt, error) {
	var result *Attempt
	err := db.learningTx(ctx, student, "student", func(tx pgx.Tx) error {
		attempt, assignment, err := attemptForActor(ctx, tx, student, attemptID)
		if err != nil {
			return err
		}
		if assignment.StudentID != student {
			return ErrNotFound
		}
		if attempt.Status != "draft" {
			result = attempt
			return nil
		}
		if err := runnableAssignment(ctx, tx, assignment); err != nil {
			return err
		}
		if err := autoGradeSubmitted(ctx, tx, attempt.ID); err != nil {
			return err
		}
		result, err = scanAttempt(tx.QueryRow(ctx, `WITH submission_time AS (SELECT clock_timestamp() AS at)
		UPDATE attempts SET status='submitted',submitted_at=s.at,
		is_late=(SELECT due_at IS NOT NULL AND s.at>due_at FROM assignments WHERE id=attempts.assignment_id)
		FROM submission_time s WHERE id=$1 RETURNING `+attemptColumns, attemptID))
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func teacherAttempt(ctx context.Context, tx pgx.Tx, teacher, attemptID int64) (*Attempt, error) {
	a, assignment, err := attemptForActor(ctx, tx, teacher, attemptID)
	if err != nil {
		return nil, err
	}
	if assignment.AssignedBy != teacher {
		return nil, ErrNotFound
	}
	return a, nil
}

// AutoGradeAnswer supports exact, whitespace-trimmed short answers; manual grading takes precedence.
func (db *DB) AutoGradeAnswer(ctx context.Context, teacher, attemptID, itemID int64, revision int) error {
	return db.learningTx(ctx, teacher, "teacher", func(tx pgx.Tx) error {
		attempt, err := teacherAttempt(ctx, tx, teacher, attemptID)
		if err != nil {
			return err
		}
		if attempt.Status == "reviewed" {
			return ErrAttemptLocked
		}
		var actual int
		var typ, expected string
		err = tx.QueryRow(ctx, "SELECT a.answer_revision,i.task_type_snapshot,COALESCE(i.answer_snapshot,'') FROM attempt_answers a JOIN worksheet_items i ON i.id=a.worksheet_item_id WHERE a.attempt_id=$1 AND i.id=$2", attemptID, itemID).Scan(&actual, &typ, &expected)
		if err != nil {
			return worksheetError(err)
		}
		if actual != revision {
			return ErrRevisionConflict
		}
		if typ != "short_answer" || strings.TrimSpace(expected) == "" {
			return ErrValidation
		}
		_, err = tx.Exec(ctx, `UPDATE attempt_answers a SET
   is_correct=(btrim(COALESCE(a.answer_text,''))<>'' AND btrim(a.answer_text)=btrim(i.answer_snapshot)),
   score=CASE WHEN btrim(COALESCE(a.answer_text,''))<>'' AND btrim(a.answer_text)=btrim(i.answer_snapshot) THEN i.max_score ELSE 0 END,
   grade_status='auto',graded_revision=a.answer_revision,graded_by=$3,teacher_comment=NULL,updated_at=now()
   FROM worksheet_items i WHERE a.attempt_id=$1 AND a.worksheet_item_id=$2 AND i.id=a.worksheet_item_id AND a.grade_status<>'manual'`, attemptID, itemID, teacher)
		return err
	})
}

func scoreMinor(score string) (int64, error) {
	if !worksheetScore.MatchString(score) {
		return 0, ErrValidation
	}
	parts := strings.Split(score, ".")
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, ErrValidation
	}
	fraction := "00"
	if len(parts) == 2 {
		fraction = (parts[1] + "00")[:2]
	}
	minor, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		return 0, ErrValidation
	}
	return whole*100 + minor, nil
}

// ReviewAnswer also permits a teacher to override an automatically graded short answer.
func (db *DB) ReviewAnswer(ctx context.Context, teacher, attemptID, itemID int64, revision int, score, comment string) error {
	score = strings.TrimSpace(score)
	comment = strings.TrimSpace(comment)
	points, err := scoreMinor(score)
	if err != nil || utf8.RuneCountInString(comment) > 5000 {
		return ErrValidation
	}
	return db.learningTx(ctx, teacher, "teacher", func(tx pgx.Tx) error {
		attempt, err := teacherAttempt(ctx, tx, teacher, attemptID)
		if err != nil {
			return err
		}
		if attempt.Status != "submitted" {
			return ErrAttemptLocked
		}
		var actual int
		var maximum string
		err = tx.QueryRow(ctx, "SELECT a.answer_revision,i.max_score::text FROM attempt_answers a JOIN worksheet_items i ON i.id=a.worksheet_item_id WHERE a.attempt_id=$1 AND i.id=$2", attemptID, itemID).Scan(&actual, &maximum)
		if err != nil {
			return worksheetError(err)
		}
		if actual != revision {
			return ErrRevisionConflict
		}
		max, err := scoreMinor(maximum)
		if err != nil {
			return err
		}
		if points > max {
			return fmt.Errorf("%w: score exceeds maximum", ErrValidation)
		}
		_, err = tx.Exec(ctx, `UPDATE attempt_answers SET score=$3,is_correct=$4,grade_status='manual',
   graded_revision=answer_revision,graded_by=$5,teacher_comment=$6,updated_at=now()
   WHERE attempt_id=$1 AND worksheet_item_id=$2`, attemptID, itemID, score, points == max, teacher, comment)
		return err
	})
}

func (db *DB) PublishAttemptResult(ctx context.Context, teacher, attemptID int64) error {
	return db.learningTx(ctx, teacher, "teacher", func(tx pgx.Tx) error {
		attempt, err := teacherAttempt(ctx, tx, teacher, attemptID)
		if err != nil {
			return err
		}
		if attempt.Status == "reviewed" {
			return nil
		}
		if attempt.Status != "submitted" {
			return ErrAttemptLocked
		}
		var pending bool
		err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM attempt_answers WHERE attempt_id=$1 AND (grade_status='pending' OR graded_revision IS DISTINCT FROM answer_revision))", attemptID).Scan(&pending)
		if err != nil {
			return err
		}
		if pending {
			return ErrIncompleteGrading
		}
		_, err = tx.Exec(ctx, "UPDATE attempts SET status='reviewed',result_published_at=now() WHERE id=$1", attemptID)
		return err
	})
}

// GetAttemptAnswers is a student-safe projection, not the internal answer model.
func (db *DB) GetAttemptAnswers(ctx context.Context, student, attemptID int64) ([]*StudentAnswer, error) {
	result := make([]*StudentAnswer, 0)
	err := db.learningTx(ctx, student, "student", func(tx pgx.Tx) error {
		attempt, assignment, err := attemptForActor(ctx, tx, student, attemptID)
		if err != nil {
			return err
		}
		if assignment.StudentID != student {
			return ErrNotFound
		}
		rows, err := tx.Query(ctx, `SELECT i.id,i.position,i.content_snapshot,i.max_score::text,COALESCE(a.answer_text,''),a.answer_data,
   a.answer_revision,a.is_correct,a.score::text,COALESCE(a.teacher_comment,'')
   FROM attempt_answers a JOIN worksheet_items i ON i.id=a.worksheet_item_id WHERE a.attempt_id=$1 ORDER BY i.position`, attemptID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			answer := new(StudentAnswer)
			var correct *bool
			var score *string
			var comment string
			if err := rows.Scan(&answer.WorksheetItemID, &answer.Position, &answer.Content, &answer.MaxScore, &answer.Text, &answer.Data, &answer.AnswerRevision, &correct, &score, &comment); err != nil {
				return err
			}
			if attempt.ResultPublishedAt != nil && correct != nil && score != nil {
				answer.Result = &AnswerGrade{IsCorrect: *correct, Score: *score, Comment: comment}
			}
			result = append(result, answer)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// GetTeacherLiveState reads saved answers with preliminary grading; transport (SSE/WS) is separate.
func (db *DB) GetTeacherLiveState(ctx context.Context, teacher, attemptID int64) ([]*TeacherAnswer, error) {
	result := make([]*TeacherAnswer, 0)
	err := db.learningTx(ctx, teacher, "teacher", func(tx pgx.Tx) error {
		if _, err := teacherAttempt(ctx, tx, teacher, attemptID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT i.id,i.position,COALESCE(a.answer_text,''),a.answer_data,a.answer_revision,
   a.grade_status,a.graded_revision,a.is_correct,a.score::text,COALESCE(a.teacher_comment,''),
   i.task_type_snapshot,COALESCE(i.answer_snapshot,'')
   FROM attempt_answers a JOIN worksheet_items i ON i.id=a.worksheet_item_id WHERE a.attempt_id=$1 ORDER BY i.position`, attemptID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			a := new(TeacherAnswer)
			var typ, expected string
			if err := rows.Scan(&a.WorksheetItemID, &a.Position, &a.Text, &a.Data, &a.AnswerRevision, &a.GradeStatus, &a.GradedRevision, &a.IsCorrect, &a.Score, &a.Comment, &typ, &expected); err != nil {
				return err
			}
			state := -1
			if a.Text == "" && a.Data == nil {
				a.LiveState = &state
			} else if a.IsCorrect != nil {
				state = 0
				if *a.IsCorrect {
					state = 1
				}
				a.LiveState = &state
			} else if typ == "short_answer" && strings.TrimSpace(expected) != "" {
				state = 0
				if strings.TrimSpace(a.Text) == strings.TrimSpace(expected) {
					state = 1
				}
				a.LiveState = &state
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
