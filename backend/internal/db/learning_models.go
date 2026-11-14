package db

import (
	"errors"
	"time"
)

var (
	ErrWorkClosed        = errors.New("db: assignment closed or unavailable")
	ErrAttemptLocked     = errors.New("db: attempt is not editable")
	ErrRevisionConflict  = errors.New("db: answer revision changed")
	ErrAttemptLimit      = errors.New("db: assignment attempt limit reached")
	ErrIncompleteGrading = errors.New("db: some answers have not been graded")
)

type Assignment struct {
	MaxAttempts    int
	ID             int64
	WorksheetID    int64
	StudentID      int64
	AssignedBy     int64
	GroupID        *int64
	DistributionID string
	DueAt          *time.Time
	Status         string
	AssignedAt     time.Time
}

type Attempt struct {
	IsLate            bool
	ID                int64
	AssignmentID      int64
	WorksheetID       int64
	AttemptNo         int
	Status            string
	StartedAt         time.Time
	SubmittedAt       *time.Time
	ResultPublishedAt *time.Time
}

// AnswerInput currently supports one answer per item. Empty Text clears an answer.
type AnswerInput struct {
	WorksheetItemID  int64
	Text             string
	Data             map[string]any
	ExpectedRevision int
}

type SavedAnswer struct {
	WorksheetItemID int64 `json:"worksheet_item_id"`
	AnswerRevision  int   `json:"answer_revision"`
}

type AnswerGrade struct {
	IsCorrect bool   `json:"is_correct"`
	Score     string `json:"score"`
	Comment   string `json:"comment"`
}

// StudentAnswer deliberately has no internal grading fields or reference answer.
type StudentAnswer struct {
	WorksheetItemID int64          `json:"worksheet_item_id"`
	Position        int            `json:"position"`
	Content         string         `json:"content"`
	MaxScore        string         `json:"max_score"`
	Text            string         `json:"answer_text"`
	Data            map[string]any `json:"answer_data,omitempty"`
	AnswerRevision  int            `json:"answer_revision"`
	Result          *AnswerGrade   `json:"result,omitempty"`
}

type TeacherAnswer struct {
	WorksheetItemID int64
	Position        int
	Text            string
	Data            map[string]any
	AnswerRevision  int
	GradeStatus     string
	GradedRevision  *int
	IsCorrect       *bool
	Score           *string
	Comment         string
	// -1: no answer, 0: incorrect, 1: correct, nil: pending.
	LiveState *int
}
