package db

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
	"time"
)

func boardTestOperation(index, base int64, id string) BoardOperation {
	return BoardOperation{
		ID: fmt.Sprintf("490df7fa-12ab-4dcb-aaba-%012x", index), BaseSeq: base,
		Payload: BoardPayload{Elements: []json.RawMessage{json.RawMessage(
			fmt.Sprintf(`{"id":%q,"type":"rectangle","x":%d}`, id, index%boardCoordinateLimit),
		)}},
	}
}

func boardTestImage(t *testing.T, fill color.Color) []byte {
	t.Helper()
	var data bytes.Buffer
	pixel := image.NewRGBA(image.Rect(0, 0, 1, 1))
	pixel.Set(0, 0, fill)
	if err := png.Encode(&data, pixel); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestBoardCompactionPreservesRecentDeletions(t *testing.T) {
	active := map[string]bool{"live": true}
	versions := map[string]int64{"live": 1, "old": 1, "recent": 5}
	if got := compactBoardRevisions(versions, active, 3); got != 3 || versions["live"] != 1 || versions["old"] != 0 || versions["recent"] != 5 {
		t.Fatalf("floor=%d, versions=%v", got, versions)
	}
	for i := range maxBoardTombstones + 2 {
		versions[fmt.Sprintf("removed-%d", i)] = int64(i + 4)
	}
	if got := compactBoardRevisions(versions, active, 3); got != 5 || len(versions) != maxBoardTombstones+1 {
		t.Fatalf("floor=%d, retained revisions=%d", got, len(versions))
	}
}

func TestValidateBoardFileRejectsDisguisedAndOversizeContent(t *testing.T) {
	imageBytes := boardTestImage(t, color.Black)
	if err := validateBoardFile("file_1", "image/png", imageBytes); err != nil {
		t.Fatal(err)
	}
	for _, input := range []struct {
		id, mime string
		data     []byte
	}{
		{"../file", "image/png", imageBytes},
		{"file", "image/jpeg", imageBytes},
		{"file", "image/png", []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)},
		{"file", "image/png", make([]byte, BoardFileLimit+1)},
	} {
		if err := validateBoardFile(input.id, input.mime, input.data); !errors.Is(err, ErrValidation) {
			t.Errorf("invalid file %q/%q accepted: %v", input.id, input.mime, err)
		}
	}
}

func TestBoardObjectKeysMatchBrowserAndLimitSurvivesDeletion(t *testing.T) {
	if _, err := parseBoardElement(json.RawMessage(`{"ID":"upper","type":"rectangle"}`)); !errors.Is(err, ErrValidation) {
		t.Fatalf("case-insensitive object id accepted: %v", err)
	}
	previous := make([]json.RawMessage, maxBoardElements)
	for i := range previous {
		previous[i] = json.RawMessage(fmt.Sprintf(`{"id":"e%d","type":"ellipse"}`, i))
	}
	changes, err := parseBoardChanges([]json.RawMessage{json.RawMessage(`{"id":"another","type":"ellipse"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := applyElements(previous, changes); !errors.Is(err, ErrValidation) {
		t.Fatalf("exceeded live object limit: %v", err)
	}
	deletion, err := parseBoardElement(json.RawMessage(`{"id":"e0","type":"ellipse","isDeleted":true}`))
	if err != nil {
		t.Fatal(err)
	}
	changes = append(changes, deletion)
	result, active, err := applyElements(previous, changes)
	if err != nil || len(result) != maxBoardElements || !active["another"] || active["e0"] {
		t.Fatalf("deletion did not free space: %d elements, %v", len(result), err)
	}
}

func TestBoardGeometryRejectsMalformedCoordinatesAndLinks(t *testing.T) {
	for _, attributes := range []string{
		`"link": "javascript:alert(1)"`,
		`"link": "java\u000ascript:alert(1)"`,
		`"link": "data:text/html,content"`,
		`"link": ["https://example.test"]`,
		`"link": "/\\example.test"`,
		`"x": "10"`,
		`"y": 1e200`,
		`"width": -1`,
		`"height": null`,
		`"angle": 1e20`,
		`"points": [[0, 0], [2]]`,
		`"points": [[0, null]]`,
		`"points": [{"x": 0, "y": 0}]`,
		`"pressures": [0.4, 2]`,
	} {
		_, err := parseBoardChanges([]json.RawMessage{json.RawMessage(`{"id":"stroke","type":"freedraw",` + attributes + `}`)})
		if !errors.Is(err, ErrValidation) {
			t.Errorf("accepted %s, err=%v", attributes, err)
		}
	}
	for _, link := range []string{`null`, `""`, `"https://example.test/path?example=5"`, `"/student/"`} {
		data := json.RawMessage(`{"id":"stroke","type":"freedraw","x":-123,"y":567,"width":0,"height":1,"angle":3.14,` +
			`"points":[[0,0],[0.5,1]],"pressures":[0.25,0.7],"link":` + link + `}`)
		if _, err := parseBoardChanges([]json.RawMessage{data}); err != nil {
			t.Errorf("valid geometry with link %s rejected: %v", link, err)
		}
	}
}

func TestBoardStorageIntegration(t *testing.T) {
	f := newBlock1Fixture(t)
	teacher := f.newUser("teacher", "board-teacher")
	otherTeacher := f.newUser("teacher", "board-other-teacher")
	var taskIDs []int64
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, query := range []string{
			`DELETE FROM boards WHERE id IN (
 SELECT b.board_id FROM board_links b JOIN worksheets w ON w.id=b.worksheet_id WHERE w.teacher_id=$1)`,
			"DELETE FROM attempt_answers WHERE worksheet_id IN (SELECT id FROM worksheets WHERE teacher_id=$1)",
			"DELETE FROM attempts WHERE worksheet_id IN (SELECT id FROM worksheets WHERE teacher_id=$1)",
			"DELETE FROM assignments WHERE assigned_by=$1",
			"DELETE FROM worksheets WHERE teacher_id=$1",
		} {
			if _, err := f.pool.Exec(ctx, query, teacher.ID); err != nil {
				t.Error(err)
			}
		}
		if _, err := f.pool.Exec(ctx, "DELETE FROM tasks WHERE id=ANY($1::bigint[])", taskIDs); err != nil {
			t.Error(err)
		}
	})
	group, err := f.db.CreateGroup(f.ctx, NewGroup{TeacherID: teacher.ID, Name: "Board group"})
	must(err)
	for i := range 10 {
		student := f.newUser("student", fmt.Sprintf("board-student-%d", i))
		must(f.db.LinkTeacherStudent(f.ctx, teacher.ID, student.ID))
		must(f.db.AddGroupMember(f.ctx, group.ID, student.ID))
	}
	makeTask := func(exam string, number int, metadata map[string]any) DraftItem {
		t.Helper()
		source := fmt.Sprintf("https://example.test/board/%d/%s/%d", f.stamp, exam, number)
		added, err := f.db.AddTask(f.ctx, &Task{
			SourceURL: source, SourceSite: "test", Subject: "math", TaskNumber: number, ExamType: exam,
			TaskType: "short_answer", ContentText: source, Answer: "42", Metadata: metadata,
		})
		must(err)
		taskIDs = append(taskIDs, added.TaskID)
		return DraftItem{TaskVersionID: added.VersionID, MaxScore: "1.00"}
	}
	revision := int64(0)
	worksheet, err := f.db.SaveWorksheetDraft(f.ctx, teacher.ID, 0, SaveDraftInput{
		Title: "Board testing", Subject: "math", MaxAttempts: 2, ExpectedRevision: &revision, Publish: true,
		Items: []DraftItem{
			makeTask("oge", 19, nil),
			makeTask("ege", 5, map[string]any{"exam_part": 1}),
			makeTask("oge", 20, nil),
			makeTask("ege", 14, map[string]any{"exam_part": 2}),
			makeTask("ege", 17, nil),
		},
	})
	must(err)
	distribution := fmt.Sprintf("490df7fb-12ab-4dcb-aaba-%012x", f.stamp&0xffffffffffff)
	assignments, err := f.db.AssignWorksheetToGroup(f.ctx, teacher.ID, worksheet.ID, group.ID, distribution, nil)
	must(err)
	if len(assignments) != 10 {
		t.Fatalf("got %d group assignments", len(assignments))
	}
	type groupBoard struct {
		assignment *Assignment
		attempt    *Attempt
		id         int64
		err        error
	}
	results := make(chan groupBoard, len(assignments))
	for _, assignment := range assignments {
		go func() {
			attempt, err := f.db.StartOrResumeAttempt(f.ctx, assignment.StudentID, assignment.ID)
			if err != nil {
				results <- groupBoard{err: err}
				return
			}
			solve, err := f.db.GetSolveAttempt(f.ctx, assignment.StudentID, attempt.ID)
			if err != nil {
				results <- groupBoard{err: err}
				return
			}
			if len(solve.Tasks) != 5 {
				results <- groupBoard{err: fmt.Errorf("wrong task count: %d", len(solve.Tasks))}
				return
			}
			shared, second, third := solve.Tasks[0].BoardID, solve.Tasks[2].BoardID, solve.Tasks[3].BoardID
			if shared <= 0 || shared != solve.Tasks[1].BoardID || second <= 0 || third <= 0 ||
				shared == second || shared == third || second == third || solve.Tasks[4].BoardID != 0 {
				results <- groupBoard{err: fmt.Errorf("incorrect partition: %+v", solve.Tasks)}
				return
			}
			results <- groupBoard{assignment: assignment, attempt: attempt, id: shared}
		}()
	}
	boards := make([]groupBoard, 0, len(assignments))
	boardIDs := make(map[int64]bool)
	for range assignments {
		entry := <-results
		if entry.err == nil && boardIDs[entry.id] {
			entry.err = fmt.Errorf("group students share board %d", entry.id)
		}
		boardIDs[entry.id] = true
		boards = append(boards, entry)
	}
	for _, board := range boards {
		must(board.err)
	}
	first := boards[0]
	studentID := first.assignment.StudentID
	for _, foreign := range []int64{otherTeacher.ID, boards[1].assignment.StudentID} {
		if _, err := f.db.GetBoardMeta(f.ctx, foreign, first.id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("foreign read = %v", err)
		}
	}
	seed := f.stamp & 0xffffffffff00
	base := int64(0)
	op := boardTestOperation(seed, base, "shared-object")
	apply := func(actor int64, op BoardOperation) BoardOperationResult {
		t.Helper()
		result, err := f.db.ApplyBoardOperation(f.ctx, actor, first.id, op)
		must(err)
		return result
	}
	firstChange := apply(studentID, op)
	if firstChange.Seq != 1 || len(firstChange.Delta) != 1 || !firstChange.CanEdit {
		t.Fatalf("first write: %+v", firstChange)
	}
	if retry := apply(studentID, op); !retry.Duplicate || retry.Seq != 1 || len(retry.Elements) != 1 {
		t.Fatalf("retry: %+v", retry)
	}
	if result := apply(teacher.ID, boardTestOperation(seed+1, base, "other-object")); result.Seq != 2 || len(result.Delta) != 1 {
		t.Fatalf("disjoint changes conflict: %+v", result)
	}
	if result := apply(teacher.ID, boardTestOperation(seed+2, base, "shared-object")); result.Seq != 2 || len(result.Conflicts) != 1 || result.Conflicts[0] != "shared-object" {
		t.Fatalf("missing conflict: %+v", result)
	}
	deleted := boardTestOperation(seed+3, 2, "shared-object")
	deleted.Payload.Elements = []json.RawMessage{json.RawMessage(`{"id":"shared-object","type":"rectangle","isDeleted":true}`)}
	if result := apply(studentID, deleted); result.Seq != 3 {
		t.Fatalf("delete failed: %+v", result)
	}
	if result := apply(teacher.ID, boardTestOperation(seed+4, 2, "shared-object")); len(result.Conflicts) != 1 {
		t.Fatalf("stale operation restored a deleted object: %+v", result)
	}
	imageOp := boardTestOperation(seed+5, 3, "drawing")
	imageOp.Payload.Elements = []json.RawMessage{json.RawMessage(`{"id":"drawing","type":"image","fileId":"picture"}`)}
	if _, err := f.db.ApplyBoardOperation(f.ctx, studentID, first.id, imageOp); !errors.Is(err, ErrValidation) {
		t.Fatalf("unuploaded image: %v", err)
	}
	imageBytes := boardTestImage(t, color.White)
	must(f.db.PutBoardFile(f.ctx, studentID, first.id, "picture", "image/png", imageBytes))
	must(f.db.PutBoardFile(f.ctx, studentID, first.id, "picture", "image/png", imageBytes))
	if err := f.db.PutBoardFile(f.ctx, teacher.ID, first.id, "picture", "image/png", boardTestImage(t, color.Black)); !errors.Is(err, ErrConflict) {
		t.Fatalf("overwrite: %v", err)
	}
	if result := apply(studentID, imageOp); result.Seq != 4 {
		t.Fatalf("image wasn't saved: %+v", result)
	}
	if _, _, err := f.db.GetBoardFile(f.ctx, boards[1].assignment.StudentID, first.id, "picture"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign file status: %v", err)
	}
	reopened, err := New(f.ctx, os.Getenv("EDTECH_TEST_DSN"))
	must(err)
	defer reopened.Close()
	mime, data, err := reopened.GetBoardFile(f.ctx, teacher.ID, first.id, "picture")
	if err != nil || mime != "image/png" || !bytes.Equal(data, imageBytes) {
		t.Fatalf("reconnected file: %s, %d bytes, %v", mime, len(data), err)
	}
	seq := int64(4)
	for i := range BoardHistoryLimit {
		current := boardTestOperation(seed+int64(i)+6, seq, "other-object")
		seq++
		if result := apply(teacher.ID, current); result.Seq != seq || len(result.Delta) != 1 {
			t.Fatalf("operation %d failed: %+v", i, result)
		}
	}
	var receipts int
	must(f.pool.QueryRow(f.ctx, "SELECT count(*) FROM board_operations WHERE board_id=$1", first.id).Scan(&receipts))
	if receipts != BoardHistoryLimit {
		t.Fatalf("journal size=%d", receipts)
	}
	if result, err := f.db.ApplyBoardOperation(f.ctx, studentID, first.id, op); !errors.Is(err, ErrBoardHistoryExpired) || result.Seq != seq {
		t.Fatalf("old replay wasn't rejected: %+v, %v", result, err)
	}
	snapshot, err := reopened.GetBoard(f.ctx, teacher.ID, first.id)
	if err != nil || snapshot.Seq != seq || len(snapshot.Elements) != 2 {
		t.Fatalf("snapshot after reconnect: %+v %v", snapshot, err)
	}
	type editorResult struct {
		actor  int64
		op     BoardOperation
		result BoardOperationResult
		err    error
	}
	edits := make(chan editorResult, 2)
	for i, actor := range []int64{teacher.ID, studentID} {
		go func() {
			op := boardTestOperation(seed+int64(i)+BoardHistoryLimit+6, seq, "other-object")
			result, err := f.db.ApplyBoardOperation(f.ctx, actor, first.id, op)
			edits <- editorResult{actor, op, result, err}
		}()
	}
	left, right := <-edits, <-edits
	must(left.err)
	must(right.err)
	if len(right.result.Delta) == 1 {
		left, right = right, left
	}
	if len(left.result.Delta) != 1 || len(right.result.Conflicts) != 1 || right.result.Conflicts[0] != "other-object" {
		t.Fatalf("simultaneous edits: %#v, %#v", left, right)
	}
	seq++
	_, err = f.db.SubmitAttempt(f.ctx, studentID, first.attempt.ID)
	must(err)
	if retry := apply(left.actor, left.op); !retry.Duplicate || retry.CanEdit || retry.Seq != seq {
		t.Fatalf("last accepted edit cannot be confirmed after submission: %+v", retry)
	}
	meta, err := f.db.GetBoardMeta(f.ctx, teacher.ID, first.id)
	if err != nil || meta.CanEdit || meta.Seq != seq {
		t.Fatalf("submitted board isn't read-only: %+v, %v", meta, err)
	}
	if err := f.db.PutBoardFile(f.ctx, teacher.ID, first.id, "other-picture", "image/png", imageBytes); !errors.Is(err, ErrAttemptLocked) {
		t.Fatalf("write to submitted board: %v", err)
	}
	nextAttempt, err := f.db.StartOrResumeAttempt(f.ctx, studentID, first.assignment.ID)
	must(err)
	next, err := f.db.GetSolveAttempt(f.ctx, studentID, nextAttempt.ID)
	must(err)
	if len(next.Tasks) != 5 || next.Tasks[0].BoardID == first.id {
		t.Fatalf("next attempt reused a previous board: %+v", next.Tasks)
	}
	must(f.db.UnlinkTeacherStudent(f.ctx, teacher.ID, studentID))
	if _, err := f.db.GetBoard(f.ctx, teacher.ID, first.id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoked teacher can still read: %v", err)
	}
}
