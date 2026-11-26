package db

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"sync"
	"testing"
)

func TestWorksheetDraftPersistence(t *testing.T) {
	f := newBlock1Fixture(t)
	teacher := f.newUser("teacher", "draft-teacher")
	other := f.newUser("teacher", "other-teacher")
	student := f.newUser("student", "draft-student")
	var taskIDs []int64
	t.Cleanup(func() {
		ctx := context.Background()
		for _, query := range []string{
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
	makeTask := func(label, subject string) int64 {
		result, err := f.db.AddTask(f.ctx, &Task{SourceURL: fmt.Sprintf("https://example.test/drafts/%d/%s", f.stamp, label), SourceSite: "test", Subject: subject, TaskNumber: 1, ExamType: "oge", TaskType: "short_answer", ContentText: label, Answer: "42"})
		if err != nil {
			t.Fatal(err)
		}
		taskIDs = append(taskIDs, result.TaskID)
		task, err := f.db.GetTask(f.ctx, result.TaskID)
		if err != nil {
			t.Fatal(err)
		}
		return task.CurrentVersionID
	}
	first := makeTask("first", "math")
	second := makeTask("second", "math")
	foreign := makeTask("foreign", "other")
	revision := int64(0)
	in := SaveDraftInput{Title: "Saved draft", Subject: "math", MaxAttempts: 2, ExpectedRevision: &revision, Items: []DraftItem{{TaskVersionID: second, MaxScore: "2.50"}, {TaskVersionID: first, MaxScore: "1.00"}}}
	saved, err := f.db.SaveWorksheetDraft(f.ctx, teacher.ID, 0, in)
	if err != nil {
		t.Fatal(err)
	}
	revision = saved.Revision
	// A separate connection reads the persisted draft without browser state.
	reconnected, err := New(f.ctx, os.Getenv("EDTECH_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer reconnected.Close()
	read, items, err := reconnected.GetWorksheetDraft(f.ctx, teacher.ID, saved.ID)
	if err != nil || len(items) != 2 || items[0].TaskVersionID != second || items[0].MaxScore != "2.50" || read.MaxAttempts != 2 || read.Revision != revision {
		t.Fatalf("reload: %+v %+v %v", read, items, err)
	}
	if _, _, err := f.db.GetWorksheetDraft(f.ctx, other.ID, saved.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign read: %v", err)
	}
	if _, err := f.db.SaveWorksheetDraft(f.ctx, other.ID, saved.ID, in); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign save: %v", err)
	}
	if _, err := f.db.SaveWorksheetDraft(f.ctx, student.ID, 0, in); err == nil {
		t.Fatal("student created draft")
	}
	in.Items = []DraftItem{{TaskVersionID: foreign, MaxScore: "1.00"}}
	if _, err := f.db.SaveWorksheetDraft(f.ctx, teacher.ID, saved.ID, in); !errors.Is(err, ErrValidation) {
		t.Fatalf("wrong subject: %v", err)
	}
	// Encoding fails after the old items have been deleted; transaction must restore them.
	in.Items = []DraftItem{{TaskVersionID: first, MaxScore: "1.00", Settings: map[string]any{"bad": math.NaN()}}}
	if _, err := f.db.SaveWorksheetDraft(f.ctx, teacher.ID, saved.ID, in); err == nil {
		t.Fatal("expected failed insert")
	}
	read, items, err = f.db.GetWorksheetDraft(f.ctx, teacher.ID, saved.ID)
	if err != nil || read.Revision != revision || len(items) != 2 || items[0].TaskVersionID != second {
		t.Fatalf("rollback lost draft: %+v %+v %v", read, items, err)
	}
	in.Items = []DraftItem{{TaskVersionID: first, MaxScore: "1.00"}, {TaskVersionID: second, MaxScore: "2.50"}}
	in.Title = "Updated draft"
	// Two editors with the same revision: exactly one commits.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.db.SaveWorksheetDraft(f.ctx, teacher.ID, saved.ID, in)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	success, conflicts := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrRevisionConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("concurrent saves: success=%d conflicts=%d", success, conflicts)
	}
	read, items, err = f.db.GetWorksheetDraft(f.ctx, teacher.ID, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if items[0].TaskVersionID != first || read.Title != "Updated draft" {
		t.Fatal("update not persisted")
	}
	revision = read.Revision
	// Legacy item APIs must invalidate the editor's revision too.
	oldItems, err := f.db.ListWorksheetItems(f.ctx, teacher.ID, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.db.UpdateWorksheetItem(f.ctx, teacher.ID, saved.ID, oldItems[0].ID, "3.00", nil); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.SaveWorksheetDraft(f.ctx, teacher.ID, saved.ID, in); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("legacy edit undetected: %v", err)
	}
	read, _, err = f.db.GetWorksheetDraft(f.ctx, teacher.ID, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	revision = read.Revision
	in.Publish = true
	published, err := f.db.SaveWorksheetDraft(f.ctx, teacher.ID, saved.ID, in)
	if err != nil || published.ID != saved.ID || published.Status != "published" {
		t.Fatalf("publish: %+v %v", published, err)
	}
	revision = published.Revision
	if _, err = f.db.SaveWorksheetDraft(f.ctx, teacher.ID, saved.ID, in); !errors.Is(err, ErrWorksheetLocked) {
		t.Fatalf("published edit: %v", err)
	}
	if err = f.db.LinkTeacherStudent(f.ctx, teacher.ID, student.ID); err != nil {
		t.Fatal(err)
	}
	assignment, err := f.db.AssignWorksheet(f.ctx, teacher.ID, published.ID, student.ID, "dc0d2340-3d4c-4d14-bd3f-123456789012", nil)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := f.db.StartOrResumeAttempt(f.ctx, student.ID, assignment.ID)
	if err != nil {
		t.Fatal(err)
	}
	answers, err := f.db.GetAttemptAnswers(f.ctx, student.ID, attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	input := AnswerInput{WorksheetItemID: answers[0].WorksheetItemID, Text: "42", ExpectedRevision: 0}
	answer, err := f.db.SaveAnswer(f.ctx, student.ID, attempt.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	input.Text = "43"
	if _, err = f.db.SaveAnswer(f.ctx, student.ID, attempt.ID, input); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale answer: %v", err)
	}
	answers, err = reconnected.GetAttemptAnswers(f.ctx, student.ID, attempt.ID)
	if err != nil || answers[0].Text != "42" || answers[0].AnswerRevision != answer.AnswerRevision {
		t.Fatalf("answer persistence: %+v %v", answers, err)
	}
	if _, err = f.db.SubmitAttempt(f.ctx, student.ID, attempt.ID); err != nil {
		t.Fatal(err)
	}
	input.ExpectedRevision = answer.AnswerRevision
	if _, err = f.db.SaveAnswer(f.ctx, student.ID, attempt.ID, input); !errors.Is(err, ErrAttemptLocked) {
		t.Fatalf("submitted edit: %v", err)
	}
}
