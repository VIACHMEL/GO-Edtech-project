package db

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	BoardMessageLimit  = 512 << 10
	BoardHistoryLimit  = 512
	maxBoardElements   = 2000
	maxBoardTombstones = 4000
)

var ErrBoardHistoryExpired = errors.New("board operation revision has expired")

type BoardMeta struct {
	Seq     int64 `json:"seq"`
	CanEdit bool  `json:"can_edit"`
}

type BoardSnapshot struct {
	Seq      int64             `json:"seq"`
	Elements []json.RawMessage `json:"elements"`
	CanEdit  bool              `json:"can_edit"`
}

type BoardOperation struct {
	ID      string       `json:"op_id"`
	BaseSeq int64        `json:"base_seq"`
	Payload BoardPayload `json:"payload"`
}

type BoardOperationResult struct {
	BoardSnapshot
	Delta     []json.RawMessage
	Conflicts []string
	Duplicate bool
}

type BoardPayload struct {
	Elements []json.RawMessage `json:"elements"`
}

type SolveAttempt struct {
	ID        int64
	Title     string
	Status    string
	CanAnswer bool
	IsTeacher bool
	Tasks     []SolveTask
}

type SolveTask struct {
	ItemID, BoardID int64
	VersionID       int64
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
		result.IsTeacher = actor == assignment.AssignedBy
		if err := tx.QueryRow(ctx, "SELECT title FROM worksheets WHERE id=$1", a.WorksheetID).Scan(&result.Title); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT i.id,i.task_version_id,i.position,i.exam_part_snapshot,COALESCE(i.content_snapshot,''),
    COALESCE(ans.answer_text,''),ans.answer_revision
FROM attempt_answers ans JOIN worksheet_items i ON i.id=ans.worksheet_item_id
WHERE ans.attempt_id=$1 ORDER BY i.position`, a.ID)
		if err != nil {
			return err
		}
		tasks, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (SolveTask, error) {
			var t SolveTask
			err := row.Scan(&t.ItemID, &t.VersionID, &t.Position, &t.Part, &t.Prompt, &t.Answer, &t.Revision)
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

func lockBoardAttempt(ctx context.Context, tx pgx.Tx, actor, boardID int64) (bool, error) {
	var attemptID int64
	if err := tx.QueryRow(ctx, "SELECT attempt_id FROM board_links WHERE board_id=$1", boardID).Scan(&attemptID); err != nil {
		return false, err
	}
	a, assignment, err := attemptForActor(ctx, tx, actor, attemptID)
	if err != nil {
		return false, err
	}
	return boardEditable(ctx, tx, a, assignment)
}

// The read path checks the same assignment, relationship and users without
// taking write locks. Mutations use lockBoardAttempt to serialize with submit.
const boardReadAccess = `
FROM boards b
JOIN board_links l ON l.board_id=b.id
JOIN attempts p ON p.id=l.attempt_id
JOIN assignments a ON a.id=p.assignment_id
JOIN users actor ON actor.id=$2 AND actor.status='active' AND actor.role IN ('teacher','student')
JOIN users student ON student.id=a.student_id
JOIN users teacher ON teacher.id=a.assigned_by
LEFT JOIN teacher_students ts ON ts.teacher_id=a.assigned_by AND ts.student_id=a.student_id AND ts.status='active'`

const boardReadFilter = `
WHERE b.id=$1 AND (actor.id=a.student_id OR (actor.id=a.assigned_by AND ts.teacher_id IS NOT NULL))`

const boardCanEdit = `p.status='draft' AND a.status='active' AND ts.teacher_id IS NOT NULL
AND student.status='active' AND student.role='student' AND teacher.status='active' AND teacher.role='teacher'`

func (db *DB) GetBoardMeta(ctx context.Context, actor, boardID int64) (BoardMeta, error) {
	var result BoardMeta
	err := db.Pool.QueryRow(ctx, "SELECT b.snapshot_seq,"+boardCanEdit+boardReadAccess+boardReadFilter,
		boardID, actor).Scan(&result.Seq, &result.CanEdit)
	return result, worksheetError(err)
}

func (db *DB) GetBoard(ctx context.Context, actor, boardID int64) (BoardSnapshot, error) {
	var result BoardSnapshot
	err := db.Pool.QueryRow(ctx, "SELECT b.snapshot_seq,b.snapshot,"+boardCanEdit+boardReadAccess+boardReadFilter,
		boardID, actor).Scan(&result.Seq, &result.Elements, &result.CanEdit)
	return result, worksheetError(err)
}

func (db *DB) ApplyBoardOperation(ctx context.Context, actor, boardID int64, op BoardOperation) (BoardOperationResult, error) {
	var operationID pgtype.UUID
	if operationID.Scan(op.ID) != nil || !operationID.Valid || op.BaseSeq < 0 || len(op.Payload.Elements) == 0 || len(op.Payload.Elements) > maxBoardElements {
		return BoardOperationResult{}, ErrValidation
	}
	payload, err := json.Marshal(op.Payload)
	if err != nil || len(payload) > BoardMessageLimit {
		return BoardOperationResult{}, ErrValidation
	}
	changes, err := parseBoardChanges(op.Payload.Elements)
	if err != nil {
		return BoardOperationResult{}, err
	}
	var result BoardOperationResult
	err = db.learningTx(ctx, actor, "", func(tx pgx.Tx) error {
		editable, err := lockBoardAttempt(ctx, tx, actor, boardID)
		if err != nil {
			return err
		}
		var floor int64
		var versions map[string]int64
		var matches *bool
		result.CanEdit = editable
		err = tx.QueryRow(ctx, `SELECT b.snapshot_seq,b.snapshot,b.history_floor,b.element_versions,
 (SELECT board_id=$1 AND author_id=$2 AND base_seq=$4 AND payload_digest=sha256(convert_to($5::jsonb::text,'UTF8'))
  FROM board_operations WHERE op_id=$3)
FROM boards b WHERE b.id=$1 FOR UPDATE OF b`, boardID, actor, operationID, op.BaseSeq, payload).
			Scan(&result.Seq, &result.Elements, &floor, &versions, &matches)
		if err != nil {
			return err
		}
		if matches != nil {
			if *matches {
				result.Duplicate = true
				return nil
			}
			return ErrConflict
		}
		if !result.CanEdit {
			return ErrAttemptLocked
		}
		if op.BaseSeq < floor {
			return ErrBoardHistoryExpired
		}
		if op.BaseSeq > result.Seq {
			return ErrValidation
		}
		for _, change := range changes {
			if versions[change.ID] > op.BaseSeq {
				result.Conflicts = append(result.Conflicts, change.ID)
			}
		}
		if len(result.Conflicts) > 0 {
			return nil
		}
		if err := checkBoardImages(ctx, tx, boardID, changes); err != nil {
			return err
		}
		next, active, err := applyElements(result.Elements, changes)
		if err != nil {
			return err
		}
		snapshot, err := json.Marshal(next)
		if err != nil || len(snapshot) > BoardMessageLimit {
			return ErrValidation
		}
		seq := result.Seq + 1
		for _, change := range changes {
			versions[change.ID] = seq
		}
		journalFloor := seq - BoardHistoryLimit
		floor = compactBoardRevisions(versions, active, max(floor, journalFloor))
		tag, err := tx.Exec(ctx, `WITH accepted AS (
 INSERT INTO board_operations(board_id,seq,op_id,author_id,base_seq,payload_digest)
 VALUES($1,$2,$3,$4,$5,sha256(convert_to($6::jsonb::text,'UTF8')))
 ON CONFLICT (op_id) DO NOTHING RETURNING board_id
), pruned AS (
 DELETE FROM board_operations
 WHERE board_id IN (SELECT board_id FROM accepted) AND seq<=$10
)
UPDATE boards SET snapshot=$7,snapshot_seq=$2,element_versions=$8,history_floor=$9,updated_at=now()
WHERE id IN (SELECT board_id FROM accepted)`, boardID, seq, operationID, actor, op.BaseSeq, payload,
			snapshot, versions, floor, journalFloor)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrConflict
		}
		result.Seq = seq
		result.Elements = nil
		result.Delta = op.Payload.Elements
		return nil
	})
	return result, err
}

type boardElement struct {
	ID      string
	Type    string
	FileID  string
	Deleted bool
	JSON    json.RawMessage
	fields  map[string]json.RawMessage
}

func parseBoardElement(raw json.RawMessage) (boardElement, error) {
	// The browser and PostgreSQL treat JSON keys as case sensitive. Decode the
	// fields used for authorization and conflict detection with the same rule.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return boardElement{}, ErrValidation
	}
	var element boardElement
	if json.Unmarshal(fields["id"], &element.ID) != nil || len(element.ID) == 0 || len(element.ID) > 128 ||
		strings.ContainsRune(element.ID, '\x00') || json.Unmarshal(fields["type"], &element.Type) != nil {
		return boardElement{}, ErrValidation
	}
	if data, present := fields["isDeleted"]; present && json.Unmarshal(data, &element.Deleted) != nil {
		return boardElement{}, ErrValidation
	}
	if data, present := fields["fileId"]; present && json.Unmarshal(data, &element.FileID) != nil {
		return boardElement{}, ErrValidation
	}
	switch element.Type {
	case "rectangle", "diamond", "ellipse", "arrow", "line", "freedraw", "text", "frame", "magicframe", "image":
		element.JSON = raw
		element.fields = fields
		return element, nil
	default:
		return boardElement{}, ErrValidation
	}
}

func parseBoardChanges(raw []json.RawMessage) ([]boardElement, error) {
	changes := make([]boardElement, 0, len(raw))
	seen := make(map[string]bool, len(raw))
	for _, data := range raw {
		element, err := parseBoardElement(data)
		if err != nil {
			return nil, err
		}
		if seen[element.ID] || !validBoardGeometry(element) {
			return nil, ErrValidation
		}
		seen[element.ID] = true
		changes = append(changes, element)
	}
	return changes, nil
}

func applyElements(previous []json.RawMessage, changes []boardElement) ([]json.RawMessage, map[string]bool, error) {
	next := make([]json.RawMessage, 0, len(previous))
	active := make(map[string]bool, len(previous))
	patch := make(map[string]boardElement, len(changes))
	for _, change := range changes {
		patch[change.ID] = change
	}
	for _, data := range previous {
		element, err := parseBoardElement(data)
		if err != nil {
			return nil, nil, err
		}
		if updated, found := patch[element.ID]; found {
			delete(patch, element.ID)
			element = updated
		}
		if !element.Deleted {
			active[element.ID] = true
			next = append(next, element.JSON)
		}
	}
	for _, change := range changes {
		if _, found := patch[change.ID]; found && !change.Deleted {
			active[change.ID] = true
			next = append(next, change.JSON)
		}
	}
	if len(next) > maxBoardElements {
		return nil, nil, ErrValidation
	}
	return next, active, nil
}

// Once the recovery floor advances, older operations can never resurrect a
// deleted object, so its revision no longer needs to be kept.
func compactBoardRevisions(versions map[string]int64, active map[string]bool, floor int64) int64 {
	if len(versions)-len(active) > maxBoardTombstones {
		removed := make([]int64, 0, len(versions)-len(active))
		for id, seq := range versions {
			if !active[id] {
				removed = append(removed, seq)
			}
		}
		sort.Slice(removed, func(i, j int) bool { return removed[i] > removed[j] })
		floor = max(floor, removed[maxBoardTombstones])
	}
	for id, seq := range versions {
		if !active[id] && seq <= floor {
			delete(versions, id)
		}
	}
	return floor
}
