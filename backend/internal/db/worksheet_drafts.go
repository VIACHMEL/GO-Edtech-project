package db

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type DraftItem struct {
	TaskVersionID int64          `json:"task_version_id"`
	MaxScore      string         `json:"max_score"`
	Settings      map[string]any `json:"settings"`
}

type DraftTask struct {
	DraftItem
	ID          int64  `json:"id"`
	Subject     string `json:"subject"`
	TaskNumber  int    `json:"task_number"`
	TaskType    string `json:"task_type"`
	ContentText string `json:"content_text"`
}

type SaveDraftInput struct {
	Title            string      `json:"title"`
	Subject          string      `json:"subject"`
	MaxAttempts      int         `json:"max_attempts"`
	ExpectedRevision *int64      `json:"expected_revision"`
	Items            []DraftItem `json:"items"`
	Publish          bool        `json:"publish"`
}

// SaveWorksheetDraft replaces an owned draft atomically. Zero ID creates a new draft.
func (db *DB) SaveWorksheetDraft(ctx context.Context, teacher, id int64, in SaveDraftInput) (*Worksheet, error) {
	if teacher <= 0 || id < 0 || in.ExpectedRevision == nil || *in.ExpectedRevision < 0 ||
		in.MaxAttempts < 1 || int64(in.MaxAttempts) > 2147483647 || len(in.Items) < 1 || len(in.Items) > 100 {
		return nil, ErrValidation
	}
	title, subject, err := worksheetText(in.Title, in.Subject)
	if err != nil {
		return nil, err
	}
	for _, item := range in.Items {
		if item.TaskVersionID <= 0 || !worksheetScore.MatchString(item.MaxScore) {
			return nil, ErrValidation
		}
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err = activeLearningUser(ctx, tx, teacher, "teacher"); err != nil {
		return nil, err
	}
	var w *Worksheet
	if id == 0 {
		if *in.ExpectedRevision != 0 {
			return nil, ErrRevisionConflict
		}
		w, err = scanWorksheet(tx.QueryRow(ctx, "INSERT INTO worksheets(teacher_id,title,subject,max_attempts) VALUES($1,$2,$3,$4) RETURNING "+worksheetColumns, teacher, title, subject, in.MaxAttempts))
	} else {
		w, err = scanWorksheet(tx.QueryRow(ctx, "SELECT "+worksheetColumns+" FROM worksheets WHERE id=$1 AND teacher_id=$2 FOR UPDATE", id, teacher))
	}
	if err != nil {
		return nil, err
	}
	if w.Status != "draft" {
		return nil, ErrWorksheetLocked
	}
	if w.Revision != *in.ExpectedRevision {
		return nil, ErrRevisionConflict
	}
	// Validate every pinned version before replacing the existing items.
	for _, item := range in.Items {
		var matches bool
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM task_versions v JOIN tasks t ON t.id=v.task_id WHERE v.id=$1 AND t.subject=$2)", item.TaskVersionID, subject).Scan(&matches); err != nil {
			return nil, err
		}
		if !matches {
			return nil, ErrValidation
		}
	}
	if _, err = tx.Exec(ctx, "DELETE FROM worksheet_items WHERE worksheet_id=$1", w.ID); err != nil {
		return nil, worksheetError(err)
	}
	for pos, item := range in.Items {
		if item.Settings == nil {
			item.Settings = map[string]any{}
		}
		if _, err = tx.Exec(ctx, "INSERT INTO worksheet_items(worksheet_id,task_version_id,position,max_score,settings) VALUES($1,$2,$3,$4,$5)", w.ID, item.TaskVersionID, pos+1, item.MaxScore, item.Settings); err != nil {
			return nil, worksheetError(err)
		}
	}
	if _, err = tx.Exec(ctx, "UPDATE worksheets SET title=$2,subject=$3,max_attempts=$4,updated_at=now() WHERE id=$1", w.ID, title, subject, in.MaxAttempts); err != nil {
		return nil, worksheetError(err)
	}
	if in.Publish {
		if err = publishWorksheet(ctx, tx, w); err != nil {
			return nil, worksheetError(err)
		}
	}
	w, err = scanWorksheet(tx.QueryRow(ctx, "SELECT "+worksheetColumns+" FROM worksheets WHERE id=$1", w.ID))
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, worksheetError(err)
	}
	return w, nil
}

// GetWorksheetDraft reads the revision and selected task versions under the same lock.
func (db *DB) GetWorksheetDraft(ctx context.Context, teacher, id int64) (*Worksheet, []DraftTask, error) {
	var result *Worksheet
	tasks := make([]DraftTask, 0)
	err := db.mutateWorksheet(ctx, teacher, id, true, func(tx pgx.Tx, w *Worksheet) error {
		result = w
		rows, err := tx.Query(ctx, `SELECT i.task_version_id,i.max_score::text,i.settings,t.id,t.subject,t.task_number,t.task_type,v.content_text
FROM worksheet_items i JOIN task_versions v ON v.id=i.task_version_id JOIN tasks t ON t.id=v.task_id
WHERE i.worksheet_id=$1 ORDER BY i.position`, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var t DraftTask
			if err := rows.Scan(&t.TaskVersionID, &t.MaxScore, &t.Settings, &t.ID, &t.Subject, &t.TaskNumber, &t.TaskType, &t.ContentText); err != nil {
				return err
			}
			tasks = append(tasks, t)
		}
		return rows.Err()
	})
	return result, tasks, err
}
