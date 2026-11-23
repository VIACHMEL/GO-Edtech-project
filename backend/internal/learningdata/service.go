package learningdata

import (
	"context"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"
)

type Service struct {
	Store *db.DB
}

func NewService(store *db.DB) *Service {
	return &Service{Store: store}
}

// выдает опубликованный вариант конкретному ученику
func (s *Service) AssignWorksheet(ctx context.Context, teacherID int64, req AssignWorksheetRequest) (AssignmentDTO, error) {
	a, err := s.Store.AssignWorksheet(ctx, teacherID, req.WorksheetID, req.StudentID, req.Distribution, req.DueAt)
	if err != nil {
		return AssignmentDTO{}, err
	}
	return ToAssignmentDTO(a), nil
}

// выдает вариант всей группе (создает назначения для каждого ученика)
func (s *Service) AssignWorksheetToGroup(ctx context.Context, teacherID int64, req AssignWorksheetToGroupRequest) ([]AssignmentDTO, error) {
	as, err := s.Store.AssignWorksheetToGroup(ctx, teacherID, req.WorksheetID, req.GroupID, req.Distribution, req.DueAt)
	if err != nil {
		return nil, err
	}
	dtos := make([]AssignmentDTO, 0, len(as))
	for _, a := range as {
		dtos = append(dtos, ToAssignmentDTO(a))
	}
	return dtos, nil
}

// возвращает список всех назначений, выданных преподавателем
func (s *Service) ListTeacherAssignments(ctx context.Context, teacherID, beforeID int64, limit int) ([]AssignmentDTO, error) {
	as, err := s.Store.ListTeacherAssignments(ctx, teacherID, beforeID, limit)
	if err != nil {
		return nil, err
	}
	dtos := make([]AssignmentDTO, 0, len(as))
	for _, a := range as {
		dtos = append(dtos, ToAssignmentDTO(a))
	}
	return dtos, nil
}

// закрывает или отменяет назначение
func (s *Service) SetAssignmentStatus(ctx context.Context, teacherID, id int64, req UpdateStatusRequest) error {
	return s.Store.SetAssignmentStatus(ctx, teacherID, id, req.Status)
}

// изменяет срок сдачи (передача nil убирает срок)
func (s *Service) UpdateAssignmentDeadline(ctx context.Context, teacherID, id int64, req UpdateDeadlineRequest) error {
	return s.Store.UpdateAssignmentDeadline(ctx, teacherID, id, req.DueAt)
}

// преподаватель открывает ученику новую попытку (в пределах лимита)
func (s *Service) ReopenAttempt(ctx context.Context, teacherID, assignmentID int64) (AttemptDTO, error) {
	att, err := s.Store.ReopenAttempt(ctx, teacherID, assignmentID)
	if err != nil {
		return AttemptDTO{}, err
	}
	return ToAttemptDTO(att), nil
}

// список всех попыток по конкретному назначению
func (s *Service) ListAssignmentAttempts(ctx context.Context, actorID, assignmentID int64) ([]AttemptDTO, error) {
	atts, err := s.Store.ListAssignmentAttempts(ctx, actorID, assignmentID)
	if err != nil {
		return nil, err
	}
	dtos := make([]AttemptDTO, 0, len(atts))
	for _, att := range atts {
		dtos = append(dtos, ToAttemptDTO(att))
	}
	return dtos, nil
}

// информация о конкретной попытке
func (s *Service) GetAttempt(ctx context.Context, actorID, id int64) (AttemptDTO, error) {
	att, err := s.Store.GetAttempt(ctx, actorID, id)
	if err != nil {
		return AttemptDTO{}, err
	}
	return ToAttemptDTO(att), nil
}

// автоматическая проверка краткого ответа
func (s *Service) AutoGradeAnswer(ctx context.Context, teacherID, attemptID, itemID int64, req AutoGradeRequest) error {
	return s.Store.AutoGradeAnswer(ctx, teacherID, attemptID, itemID, req.Revision)
}

// ручная оценка ответа преподавателем
func (s *Service) ReviewAnswer(ctx context.Context, teacherID, attemptID, itemID int64, req ReviewAnswerRequest) error {
	return s.Store.ReviewAnswer(ctx, teacherID, attemptID, itemID, req.Revision, req.Score, req.Comment)
}

// публикация результата ученику
func (s *Service) PublishAttemptResult(ctx context.Context, teacherID, attemptID int64) error {
	return s.Store.PublishAttemptResult(ctx, teacherID, attemptID)
}

// "живое" состояние ответов ученика для преподавателя
func (s *Service) GetTeacherLiveState(ctx context.Context, teacherID, attemptID int64) ([]TeacherAnswerDTO, error) {
	answers, err := s.Store.GetTeacherLiveState(ctx, teacherID, attemptID)
	if err != nil {
		return nil, err
	}
	dtos := make([]TeacherAnswerDTO, 0, len(answers))
	for _, ans := range answers {
		dtos = append(dtos, ToTeacherAnswerDTO(ans))
	}
	return dtos, nil
}

// список заданий, выданных конкретному ученику
func (s *Service) ListStudentAssignments(ctx context.Context, studentID, beforeID int64, limit int) ([]AssignmentDTO, error) {
	as, err := s.Store.ListStudentAssignments(ctx, studentID, beforeID, limit)
	if err != nil {
		return nil, err
	}
	dtos := make([]AssignmentDTO, 0, len(as))
	for _, a := range as {
		dtos = append(dtos, ToAssignmentDTO(a))
	}
	return dtos, nil
}

// ученик начинает решение (или продолжает черновик)
func (s *Service) StartOrResumeAttempt(ctx context.Context, studentID, assignmentID int64) (AttemptDTO, error) {
	att, err := s.Store.StartOrResumeAttempt(ctx, studentID, assignmentID)
	if err != nil {
		return AttemptDTO{}, err
	}
	return ToAttemptDTO(att), nil
}

// получение условий задач и сохраненных ответов для ученика
func (s *Service) GetAttemptAnswers(ctx context.Context, studentID, attemptID int64) ([]*db.StudentAnswer, error) {
	return s.Store.GetAttemptAnswers(ctx, studentID, attemptID)
}

// сохранение ответа на конкретную задачу
func (s *Service) SaveAnswer(ctx context.Context, studentID, attemptID, itemID int64, req SaveAnswerRequest) (*db.SavedAnswer, error) {
	input := db.AnswerInput{
		WorksheetItemID:  itemID,
		Text:             req.Text,
		Data:             req.Data,
		ExpectedRevision: req.ExpectedRevision,
	}
	return s.Store.SaveAnswer(ctx, studentID, attemptID, input)
}

// очистка ответа
func (s *Service) ClearAnswer(ctx context.Context, studentID, attemptID, itemID int64, revision int) (*db.SavedAnswer, error) {
	return s.Store.ClearAnswer(ctx, studentID, attemptID, itemID, revision)
}

// сдача работы (запускает авто-проверку и фиксирует время)
func (s *Service) SubmitAttempt(ctx context.Context, studentID, attemptID int64) (AttemptDTO, error) {
	att, err := s.Store.SubmitAttempt(ctx, studentID, attemptID)
	if err != nil {
		return AttemptDTO{}, err
	}
	return ToAttemptDTO(att), nil
}

// преобразует назначение из БД в DTO
func ToAssignmentDTO(a *db.Assignment) AssignmentDTO {
	return AssignmentDTO{
		ID:             a.ID,
		WorksheetID:    a.WorksheetID,
		WorksheetTitle: a.WorksheetTitle,
		StudentID:      a.StudentID,
		AssignedBy:     a.AssignedBy,
		GroupID:        a.GroupID,
		DistributionID: a.DistributionID,
		DueAt:          a.DueAt,
		Status:         a.Status,
		AssignedAt:     a.AssignedAt,
		MaxAttempts:    a.MaxAttempts,
	}
}

// преобразует попытку из БД в DTO
func ToAttemptDTO(a *db.Attempt) AttemptDTO {
	return AttemptDTO{
		ID:                a.ID,
		AssignmentID:      a.AssignmentID,
		WorksheetID:       a.WorksheetID,
		AttemptNo:         a.AttemptNo,
		Status:            a.Status,
		StartedAt:         a.StartedAt,
		SubmittedAt:       a.SubmittedAt,
		ResultPublishedAt: a.ResultPublishedAt,
		IsLate:            a.IsLate,
	}
}

// преобразует ответ для учителя из БД в DTO
func ToTeacherAnswerDTO(a *db.TeacherAnswer) TeacherAnswerDTO {
	return TeacherAnswerDTO{
		WorksheetItemID: a.WorksheetItemID,
		Position:        a.Position,
		Text:            a.Text,
		Data:            a.Data,
		AnswerRevision:  a.AnswerRevision,
		GradeStatus:     a.GradeStatus,
		GradedRevision:  a.GradedRevision,
		IsCorrect:       a.IsCorrect,
		Score:           a.Score,
		Comment:         a.Comment,
		LiveState:       a.LiveState,
	}
}
