package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	BoardMessageLimit = 512 << 10
	maxBoardElements  = 2000
)

type BoardSnapshot struct {
	Seq      int64             `json:"seq"`
	Elements []json.RawMessage `json:"elements"`
	CanEdit  bool              `json:"can_edit"`
}

type BoardOperation struct {
	ID      string       `json:"op_id"`
	Payload BoardPayload `json:"payload"`
}

type BoardPayload struct {
	Elements []json.RawMessage `json:"elements"`
}

type SolveAttempt struct {
	ID        int64
	Title     string
	Status    string
	CanAnswer bool
	Tasks     []SolveTask
}

type SolveTask struct {
	ItemID, BoardID int64
	Position        int
	Part            *int16
	Prompt, Answer  string
	Revision        int
}

func boardEditable(ctx context.Context, tx pgx.Tx, a *Attempt, assignment *Assignment) (bool, error) {
	if a.Status != "draft" || assignment.Status != "active" {
		return false, nil
	}
	if err := runnableAssignment(ctx, tx, assignment); err != nil {
		if errors.Is(err, ErrWorkClosed) {
			return false, nil
		}
		return false, err
	}
	if err := activeLearningUser(ctx, tx, assignment.StudentID, "student"); err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (db *DB) GetSolveAttempt(ctx context.Context, actor, attemptID int64) (SolveAttempt, error) {
	var result SolveAttempt
	err := db.learningTx(ctx, actor, "", func(tx pgx.Tx) error {
		a, assignment, err := attemptForActor(ctx, tx, actor, attemptID)
		if err != nil {
			return err
		}
		editable, err := boardEditable(ctx, tx, a, assignment)
		if err != nil {
			return err
		}
		result.ID, result.Status = a.ID, a.Status
		result.CanAnswer = editable && actor == assignment.StudentID
		if err := tx.QueryRow(ctx, "SELECT title FROM worksheets WHERE id=$1", a.WorksheetID).Scan(&result.Title); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT i.id,i.position,i.exam_part_snapshot,COALESCE(i.content_snapshot,''),
    COALESCE(ans.answer_text,''),ans.answer_revision
FROM attempt_answers ans JOIN worksheet_items i ON i.id=ans.worksheet_item_id
WHERE ans.attempt_id=$1 ORDER BY i.position`, a.ID)
		if err != nil {
			return err
		}
		tasks, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (SolveTask, error) {
			var t SolveTask
			err := row.Scan(&t.ItemID, &t.Position, &t.Part, &t.Prompt, &t.Answer, &t.Revision)
			return t, err
		})
		if err != nil {
			return err
		}
		var sharedID int64
		for i := range tasks {
			task := &tasks[i]
			if task.Part == nil {
				continue
			}
			if *task.Part == 1 && sharedID != 0 {
				task.BoardID = sharedID
				continue
			}
			var itemID *int64
			if *task.Part == 2 {
				itemID = &task.ItemID
			}
			task.BoardID, err = findOrCreateBoard(ctx, tx, a, assignment.StudentID, itemID)
			if err != nil {
				return err
			}
			if itemID == nil {
				sharedID = task.BoardID
			}
		}
		result.Tasks = tasks
		return nil
	})
	return result, err
}

// The attempt row is already locked, so both roles resolve the same board.
func findOrCreateBoard(ctx context.Context, tx pgx.Tx, a *Attempt, owner int64, itemID *int64) (int64, error) {
	var boardID int64
	err := tx.QueryRow(ctx, `SELECT board_id FROM board_links
WHERE attempt_id=$1 AND worksheet_item_id IS NOT DISTINCT FROM $2::bigint`, a.ID, itemID).Scan(&boardID)
	if !errors.Is(err, pgx.ErrNoRows) {
		return boardID, err
	}
	err = tx.QueryRow(ctx, `WITH created AS (INSERT INTO boards(owner_id) VALUES($1) RETURNING id)
INSERT INTO board_links(board_id,attempt_id,worksheet_id,worksheet_item_id)
SELECT id,$2,$3,$4 FROM created RETURNING board_id`, owner, a.ID, a.WorksheetID, itemID).Scan(&boardID)
	return boardID, err
}

func boardForActor(ctx context.Context, tx pgx.Tx, actor, boardID int64) (BoardSnapshot, error) {
	var attemptID int64
	if err := tx.QueryRow(ctx, "SELECT attempt_id FROM board_links WHERE board_id=$1", boardID).Scan(&attemptID); err != nil {
		return BoardSnapshot{}, err
	}
	a, assignment, err := attemptForActor(ctx, tx, actor, attemptID)
	if err != nil {
		return BoardSnapshot{}, err
	}
	editable, err := boardEditable(ctx, tx, a, assignment)
	if err != nil {
		return BoardSnapshot{}, err
	}
	result := BoardSnapshot{CanEdit: editable}
	err = tx.QueryRow(ctx, "SELECT snapshot_seq,snapshot FROM boards WHERE id=$1 FOR UPDATE", boardID).
		Scan(&result.Seq, &result.Elements)
	return result, err
}

func (db *DB) GetBoard(ctx context.Context, actor, boardID int64) (BoardSnapshot, error) {
	var result BoardSnapshot
	err := db.learningTx(ctx, actor, "", func(tx pgx.Tx) error {
		var err error
		result, err = boardForActor(ctx, tx, actor, boardID)
		return err
	})
	return result, err
}

func (db *DB) ApplyBoardOperation(ctx context.Context, actor, boardID int64, op BoardOperation) (BoardSnapshot, error) {
	var operationID pgtype.UUID
	if operationID.Scan(op.ID) != nil || !operationID.Valid || len(op.Payload.Elements) == 0 || len(op.Payload.Elements) > maxBoardElements {
		return BoardSnapshot{}, ErrValidation
	}
	payload, err := json.Marshal(op.Payload)
	if err != nil || len(payload) > BoardMessageLimit {
		return BoardSnapshot{}, ErrValidation
	}
	var result BoardSnapshot
	err = db.learningTx(ctx, actor, "", func(tx pgx.Tx) error {
		var err error
		result, err = boardForActor(ctx, tx, actor, boardID)
		if err != nil {
			return err
		}
		var matches bool
		err = tx.QueryRow(ctx, `SELECT board_id=$2 AND author_id=$3 AND payload=$4::jsonb
FROM board_operations WHERE op_id=$1`, operationID, boardID, actor, payload).Scan(&matches)
		switch {
		case err == nil && matches:
			return nil
		case err == nil:
			return ErrConflict
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		if !result.CanEdit {
			return ErrAttemptLocked
		}
		next, err := applyElements(result.Elements, op.Payload.Elements)
		if err != nil {
			return err
		}
		snapshot, err := json.Marshal(next)
		if err != nil || len(snapshot) > BoardMessageLimit {
			return ErrValidation
		}
		result.Seq++
		result.Elements = next
		tag, err := tx.Exec(ctx, `INSERT INTO board_operations(board_id,seq,op_id,author_id,payload)
VALUES($1,$2,$3,$4,$5) ON CONFLICT (op_id) DO NOTHING`, boardID, result.Seq, operationID, actor, payload)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrConflict
		}
		_, err = tx.Exec(ctx, "UPDATE boards SET snapshot=$2,snapshot_seq=$3,updated_at=now() WHERE id=$1",
			boardID, snapshot, result.Seq)
		return err
	})
	return result, err
}

func boardElementID(raw json.RawMessage) (string, error) {
	var element struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &element); err != nil || element.ID == "" || len(element.ID) > 128 {
		return "", ErrValidation
	}
	switch element.Type {
	case "rectangle", "diamond", "ellipse", "arrow", "line", "freedraw", "text", "frame", "magicframe":
		return element.ID, nil
	default:
		return "", fmt.Errorf("%w: unsupported board element", ErrValidation)
	}
}

func applyElements(previous, changes []json.RawMessage) ([]json.RawMessage, error) {
	next := append(make([]json.RawMessage, 0, len(previous)), previous...)
	positions := make(map[string]int, len(previous))
	for index, element := range previous {
		id, err := boardElementID(element)
		if err != nil {
			return nil, err
		}
		positions[id] = index
	}
	for _, element := range changes {
		id, err := boardElementID(element)
		if err != nil {
			return nil, err
		}
		if index, exists := positions[id]; exists {
			next[index] = element
		} else {
			positions[id] = len(next)
			next = append(next, element)
		}
	}
	if len(next) > maxBoardElements {
		return nil, ErrValidation
	}
	return next, nil
}
