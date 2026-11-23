package db

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrWorksheetLocked   = errors.New("db: worksheet is not a draft")
	ErrWorksheetConflict = errors.New("db: worksheet item position already exists")
)

type Worksheet struct {
	ID          int64
	TeacherID   int64
	Title       string
	Subject     string
	MaxAttempts int
	Status      string
	CreatedAt   time.Time
	PublishedAt *time.Time
	UpdatedAt   time.Time
}

type WorksheetItem struct {
	ID            int64
	WorksheetID   int64
	TaskVersionID int64
	Position      int
	// Decimal string avoids rounding scores, for example "2.50".
	MaxScore string
	Settings map[string]any
}

const worksheetColumns = "id,teacher_id,title,subject,status,created_at,published_at,updated_at,max_attempts"
const worksheetItemColumns = "id,worksheet_id,task_version_id,position,max_score::text,settings"

func scanWorksheet(row pgx.Row) (*Worksheet, error) {
	w := new(Worksheet)
	err := row.Scan(&w.ID, &w.TeacherID, &w.Title, &w.Subject, &w.Status, &w.CreatedAt, &w.PublishedAt, &w.UpdatedAt, &w.MaxAttempts)
	return w, worksheetError(err)
}

func worksheetError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var p *pgconn.PgError
	if errors.As(err, &p) {
		switch p.Code {
		case "23505":
			return fmt.Errorf("%w: %s", ErrWorksheetConflict, p.ConstraintName)
		case "23503", "23514", "22003", "22P02":
			return fmt.Errorf("%w: %s", ErrValidation, p.Message)
		}
	}
	return err
}

func worksheetText(title, subject string) (string, string, error) {
	title, subject = strings.TrimSpace(title), strings.TrimSpace(subject)
	if utf8.RuneCountInString(title) < 1 || utf8.RuneCountInString(title) > 200 ||
		utf8.RuneCountInString(subject) < 1 || utf8.RuneCountInString(subject) > 64 {
		return "", "", fmt.Errorf("%w: title (1..200) and subject (1..64) required", ErrValidation)
	}
	return title, subject, nil
}

func worksheetIDs(teacherID, worksheetID int64) error {
	if teacherID <= 0 || worksheetID <= 0 {
		return fmt.Errorf("%w: positive teacher and worksheet IDs required", ErrValidation)
	}
	return nil
}

// CreateWorksheet creates an owned draft. teacherID must come from the authenticated server context.
func (db *DB) CreateWorksheet(ctx context.Context, teacherID int64, title, subject string, maxAttempts int) (*Worksheet, error) {
	if teacherID <= 0 || maxAttempts < 1 || int64(maxAttempts) > 2147483647 {
		return nil, ErrValidation
	}
	title, subject, err := worksheetText(title, subject)
	if err != nil {
		return nil, err
	}
	return scanWorksheet(db.Pool.QueryRow(ctx, "INSERT INTO worksheets(teacher_id,title,subject,max_attempts) SELECT id,$2,$3,$4 FROM users WHERE id=$1 AND role='teacher' AND status='active' RETURNING "+worksheetColumns, teacherID, title, subject, maxAttempts))
}

// GetWorksheet hides both missing worksheets and worksheets owned by another teacher.
func (db *DB) GetWorksheet(ctx context.Context, teacherID, id int64) (*Worksheet, error) {
	if err := worksheetIDs(teacherID, id); err != nil {
		return nil, err
	}
	return scanWorksheet(db.Pool.QueryRow(ctx, "SELECT "+worksheetColumns+" FROM worksheets WHERE id=$1 AND teacher_id=$2 AND EXISTS(SELECT 1 FROM users WHERE id=$2 AND role='teacher' AND status='active')", id, teacherID))
}

// ListWorksheets returns only the teacher's worksheets, newest first, with keyset pagination.
func (db *DB) ListWorksheets(ctx context.Context, teacherID, beforeID int64, limit int) ([]*Worksheet, error) {
	if teacherID <= 0 || beforeID < 0 {
		return nil, ErrValidation
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := db.Pool.Query(ctx, "SELECT "+worksheetColumns+" FROM worksheets WHERE teacher_id=$1 AND ($2::bigint=0 OR id<$2) AND EXISTS(SELECT 1 FROM users WHERE id=$1 AND role='teacher' AND status='active') ORDER BY id DESC LIMIT $3", teacherID, beforeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]*Worksheet, 0)
	for rows.Next() {
		w, err := scanWorksheet(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, w)
	}
	return result, rows.Err()
}

func (db *DB) mutateWorksheet(ctx context.Context, teacherID, id int64, draftOnly bool, change func(pgx.Tx, *Worksheet) error) error {
	if err := worksheetIDs(teacherID, id); err != nil {
		return err
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := activeLearningUser(ctx, tx, teacherID, "teacher"); err != nil {
		return err
	}
	w, err := scanWorksheet(tx.QueryRow(ctx, "SELECT "+worksheetColumns+" FROM worksheets WHERE id=$1 AND teacher_id=$2 FOR UPDATE", id, teacherID))
	if err != nil {
		return err
	}
	if draftOnly && w.Status != "draft" {
		return ErrWorksheetLocked
	}
	if err = change(tx, w); err != nil {
		return worksheetError(err)
	}
	return worksheetError(tx.Commit(ctx))
}

// UpdateWorksheet changes draft metadata; a subject change must still match all selected tasks.
func (db *DB) UpdateWorksheet(ctx context.Context, teacherID, id int64, title, subject string, maxAttempts int) error {
	if maxAttempts < 1 || int64(maxAttempts) > 2147483647 {
		return ErrValidation
	}
	title, subject, err := worksheetText(title, subject)
	if err != nil {
		return err
	}
	return db.mutateWorksheet(ctx, teacherID, id, true, func(tx pgx.Tx, w *Worksheet) error {
		var mismatch bool
		err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM worksheet_items i JOIN task_versions v ON v.id=i.task_version_id JOIN tasks t ON t.id=v.task_id WHERE i.worksheet_id=$1 AND t.subject<>$2)", id, subject).Scan(&mismatch)
		if err != nil {
			return err
		}
		if mismatch {
			return fmt.Errorf("%w: selected tasks have another subject", ErrValidation)
		}
		_, err = tx.Exec(ctx, "UPDATE worksheets SET title=$2,subject=$3,max_attempts=$4,updated_at=now() WHERE id=$1", id, title, subject, maxAttempts)
		return err
	})
}

// PublishWorksheet freezes a nonempty draft. Repeated publication of the same worksheet is harmless.
func (db *DB) PublishWorksheet(ctx context.Context, teacherID, id int64) error {
	return db.mutateWorksheet(ctx, teacherID, id, false, func(tx pgx.Tx, w *Worksheet) error {
		if w.Status == "published" {
			return nil
		}
		if w.Status != "draft" {
			return ErrWorksheetLocked
		}
		var count int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM worksheet_items WHERE worksheet_id=$1", id).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			return fmt.Errorf("%w: cannot publish an empty worksheet", ErrValidation)
		}
		if _, err := tx.Exec(ctx, `UPDATE worksheet_items i
SET content_snapshot=v.content_text, answer_snapshot=v.answer, solution_snapshot=v.solution,
    task_type_snapshot=t.task_type,
    exam_part_snapshot=task_exam_part(t.exam_type,t.subject,t.task_number,t.metadata)
FROM task_versions v JOIN tasks t ON t.id=v.task_id
WHERE i.worksheet_id=$1 AND i.task_version_id=v.id`, id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "UPDATE worksheets SET status='published',published_at=now(),updated_at=now() WHERE id=$1", id)
		return err
	})
}

// ArchiveWorksheet removes a worksheet from active use without deleting its history.
func (db *DB) ArchiveWorksheet(ctx context.Context, teacherID, id int64) error {
	return db.mutateWorksheet(ctx, teacherID, id, false, func(tx pgx.Tx, w *Worksheet) error {
		if w.Status == "archived" {
			return nil
		}
		_, err := tx.Exec(ctx, "UPDATE worksheets SET status='archived',updated_at=now() WHERE id=$1", id)
		return err
	})
}

// CloneWorksheet copies an owned worksheet and its pinned task versions into a new draft atomically.
func (db *DB) CloneWorksheet(ctx context.Context, teacherID, id int64, title string) (*Worksheet, error) {
	var result *Worksheet
	err := db.mutateWorksheet(ctx, teacherID, id, false, func(tx pgx.Tx, w *Worksheet) error {
		name, _, err := worksheetText(title, w.Subject)
		if err != nil {
			return err
		}
		result, err = scanWorksheet(tx.QueryRow(ctx, "INSERT INTO worksheets(teacher_id,title,subject,max_attempts) VALUES($1,$2,$3,$4) RETURNING "+worksheetColumns, teacherID, name, w.Subject, w.MaxAttempts))
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "INSERT INTO worksheet_items(worksheet_id,task_version_id,position,max_score,settings) SELECT $1,task_version_id,position,max_score,settings FROM worksheet_items WHERE worksheet_id=$2", result.ID, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

var worksheetScore = regexp.MustCompile(`^[0-9]{1,4}(\.[0-9]{1,2})?$`)

// UpdateWorksheetItem changes score and settings only in an owned draft.
func (db *DB) UpdateWorksheetItem(ctx context.Context, teacherID, id, itemID int64, maxScore string, settings map[string]any) error {
	if itemID <= 0 || !worksheetScore.MatchString(maxScore) {
		return ErrValidation
	}
	if settings == nil {
		settings = map[string]any{}
	}
	return db.mutateWorksheet(ctx, teacherID, id, true, func(tx pgx.Tx, w *Worksheet) error {
		tag, err := tx.Exec(ctx, "UPDATE worksheet_items SET max_score=$3,settings=$4 WHERE worksheet_id=$1 AND id=$2", id, itemID, maxScore, settings)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		_, err = tx.Exec(ctx, "UPDATE worksheets SET updated_at=now() WHERE id=$1", id)
		return err
	})
}

// AddWorksheetItem pins a specific task version at a positive position in an owned draft.
func (db *DB) AddWorksheetItem(ctx context.Context, teacherID, id int64, item WorksheetItem) (*WorksheetItem, error) {
	if item.TaskVersionID <= 0 || item.Position <= 0 || !worksheetScore.MatchString(item.MaxScore) {
		return nil, ErrValidation
	}
	if item.Settings == nil {
		item.Settings = map[string]any{}
	}
	var result WorksheetItem
	err := db.mutateWorksheet(ctx, teacherID, id, true, func(tx pgx.Tx, w *Worksheet) error {
		var subject string
		err := tx.QueryRow(ctx, "SELECT t.subject FROM task_versions v JOIN tasks t ON t.id=v.task_id WHERE v.id=$1", item.TaskVersionID).Scan(&subject)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: task version not found", ErrValidation)
		}
		if err != nil {
			return err
		}
		if subject != w.Subject {
			return fmt.Errorf("%w: subject mismatch", ErrValidation)
		}
		err = tx.QueryRow(ctx, "INSERT INTO worksheet_items(worksheet_id,task_version_id,position,max_score,settings) VALUES($1,$2,$3,$4,$5) RETURNING "+worksheetItemColumns, id, item.TaskVersionID, item.Position, item.MaxScore, item.Settings).Scan(&result.ID, &result.WorksheetID, &result.TaskVersionID, &result.Position, &result.MaxScore, &result.Settings)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "UPDATE worksheets SET updated_at=now() WHERE id=$1", id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// ListWorksheetItems returns items only to their worksheet's owner.
func (db *DB) ListWorksheetItems(ctx context.Context, teacherID, id int64) ([]*WorksheetItem, error) {
	// A single transaction keeps ownership lookup and item read in one consistent lock scope.
	result := make([]*WorksheetItem, 0)
	err := db.mutateWorksheet(ctx, teacherID, id, false, func(tx pgx.Tx, w *Worksheet) error {
		rows, err := tx.Query(ctx, "SELECT "+worksheetItemColumns+" FROM worksheet_items WHERE worksheet_id=$1 ORDER BY position", id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			item := new(WorksheetItem)
			if err := rows.Scan(&item.ID, &item.WorksheetID, &item.TaskVersionID, &item.Position, &item.MaxScore, &item.Settings); err != nil {
				return err
			}
			result = append(result, item)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// RemoveWorksheetItem deletes an item only from an owned draft; positions may retain gaps.
func (db *DB) RemoveWorksheetItem(ctx context.Context, teacherID, id, itemID int64) error {
	if itemID <= 0 {
		return ErrValidation
	}
	return db.mutateWorksheet(ctx, teacherID, id, true, func(tx pgx.Tx, w *Worksheet) error {
		tag, err := tx.Exec(ctx, "DELETE FROM worksheet_items WHERE worksheet_id=$1 AND id=$2", id, itemID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		_, err = tx.Exec(ctx, "UPDATE worksheets SET updated_at=now() WHERE id=$1", id)
		return err
	})
}

// ReorderWorksheetItems requires every item ID exactly once and atomically assigns positions 1..N.
func (db *DB) ReorderWorksheetItems(ctx context.Context, teacherID, id int64, itemIDs []int64) error {
	seen := make(map[int64]bool, len(itemIDs))
	for _, itemID := range itemIDs {
		if itemID <= 0 || seen[itemID] {
			return ErrValidation
		}
		seen[itemID] = true
	}
	return db.mutateWorksheet(ctx, teacherID, id, true, func(tx pgx.Tx, w *Worksheet) error {
		var total, matching int
		if err := tx.QueryRow(ctx, "SELECT count(*),count(*) FILTER(WHERE id=ANY($2::bigint[])) FROM worksheet_items WHERE worksheet_id=$1", id, itemIDs).Scan(&total, &matching); err != nil {
			return err
		}
		if total != len(itemIDs) || matching != total {
			return fmt.Errorf("%w: provide all and only this worksheet's items", ErrValidation)
		}
		if _, err := tx.Exec(ctx, "SET CONSTRAINTS worksheet_items_position_key DEFERRED"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE worksheet_items i SET position=s.position FROM unnest($2::bigint[]) WITH ORDINALITY AS s(id,position) WHERE i.worksheet_id=$1 AND i.id=s.id", id, itemIDs); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "UPDATE worksheets SET updated_at=now() WHERE id=$1", id)
		return err
	})
}
