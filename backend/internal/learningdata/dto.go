package learningdata

import "time"

// информация о выданном ученику задании
type AssignmentDTO struct {
	ID int64 `json:"id"`
	WorksheetID int64 `json:"worksheet_id"`
	StudentID int64 `json:"student_id"`
	AssignedBy int64 `json:"assigned_by"`
	GroupID *int64 `json:"group_id"` // null, если выдали лично
	DistributionID string `json:"distribution_id"` // UUID операции выдачи
	DueAt *time.Time `json:"due_at"` // дедлайн (может быть null)
	Status string `json:"status"` // "active", "closed", "cancelled"
	AssignedAt time.Time `json:"assigned_at"`
	MaxAttempts int `json:"max_attempts"`
}

// конкретная попытка решения задания учеником
type AttemptDTO struct {
	ID int64 `json:"id"`
	AssignmentID int64 `json:"assignment_id"`
	WorksheetID int64 `json:"worksheet_id"`
	AttemptNo int `json:"attempt_no"`
	Status string `json:"status"` // "draft", "submitted", "reviewed"
	StartedAt time.Time `json:"started_at"`
	SubmittedAt *time.Time `json:"submitted_at"` // null, если еще черновик
	ResultPublishedAt *time.Time `json:"result_published_at"` // когда опубликована оценка
	IsLate bool `json:"is_late"` // сдано ли после дедлайна
}

// ответ ученика, каким его видит преподаватель (включая скрытые поля)
type TeacherAnswerDTO struct {
	WorksheetItemID int64 `json:"worksheet_item_id"`
	Position int `json:"position"`
	Text string `json:"text"`
	Data map[string]any `json:"data"`
	AnswerRevision int `json:"answer_revision"`
	GradeStatus string `json:"grade_status"` // "pending", "auto", "manual"
	GradedRevision *int `json:"graded_revision"`
	IsCorrect *bool `json:"is_correct"` // null, если еще не проверено
	Score *string `json:"score"` // null, если еще не проверено
	Comment string `json:"comment"`
	LiveState *int `json:"live_state"` // -1 нет ответа, 0 неверно, 1 верно, null ждет проверки
}

// запрос на выдачу задания одному ученику
type AssignWorksheetRequest struct {
	WorksheetID int64 `json:"worksheet_id"`
	StudentID int64 `json:"student_id"`
	Distribution string `json:"distribution"` // UUID для идемпотентности
	DueAt *time.Time `json:"due_at"`
}

// запрос на выдачу задания всей группе
type AssignWorksheetToGroupRequest struct {
	WorksheetID int64 `json:"worksheet_id"`
	GroupID int64 `json:"group_id"`
	Distribution string `json:"distribution"`
	DueAt *time.Time `json:"due_at"`
}

// запрос на смену статуса назначения (закрыть/отменить)
type UpdateStatusRequest struct {
	Status string `json:"status"`
}

// запрос на изменение дедлайна (передача null убирает срок)
type UpdateDeadlineRequest struct {
	DueAt *time.Time `json:"due_at"`
}

// запрос на сохранение ответа учеником
type SaveAnswerRequest struct {
	Text string `json:"text"`
	Data map[string]any `json:"data"`
	ExpectedRevision int `json:"expected_revision"` // для защиты от конфликтов
}

// запрос на ручную оценку ответа преподавателем
type ReviewAnswerRequest struct {
	Revision int `json:"revision"`
	Score string `json:"score"`
	Comment string `json:"comment"`
}

// запрос на автоматическую проверку краткого ответа
type AutoGradeRequest struct {
	Revision int `json:"revision"`
}