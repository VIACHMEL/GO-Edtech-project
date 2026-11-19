package teacherdata

import (
	"context"
	"strings"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"
)

// Service преобразует модели чтения БД в структуры для JSON-ответа.
// SQL, проверки роли преподавателя и фильтрация находятся в пакете db.
type Service struct {
	Store *db.DB
}

func NewService(store *db.DB) *Service {
	return &Service{Store: store}
}

func (s *Service) GetProfile(ctx context.Context, userID int64) (ProfileDTO, error) {
	user, err := s.Store.GetUser(ctx, userID)
	if err != nil {
		return ProfileDTO{}, err
	}
	return ToProfileDTO(*user), nil
}

// GetTeacherStudents сохраняет прежний JSON-контракт. В ответ входят только
// активные ученики; фильтры "all" и "active" сейчас равнозначны.
func (s *Service) GetTeacherStudents(ctx context.Context, teacherID int64, query, status string) (StudentListResponse, error) {
	response := StudentListResponse{Students: make([]StudentListItemDTO, 0)}
	page, err := s.Store.ListTeacherStudentsPage(ctx, teacherID, query, status)
	if err != nil {
		return response, err
	}
	response.Total = page.Total
	// Поле оставлено для совместимости, а не как отдельная категория учеников.
	response.ActiveCount = page.Total
	for _, student := range page.Students {
		response.Students = append(response.Students, ToStudentDTO(student))
	}
	return response, nil
}

// GetTeacherGroups сохраняет формат JSON, использовавшийся до переработки.
func (s *Service) GetTeacherGroups(ctx context.Context, teacherID int64, query, status string) (GroupListResponse, error) {
	response := GroupListResponse{Groups: make([]GroupListItemDTO, 0)}
	page, err := s.Store.ListTeacherGroupsPage(ctx, teacherID, query, status)
	if err != nil {
		return response, err
	}
	response.Total = page.Total
	response.ActiveCount = page.ActiveCount
	response.ArchivedCount = page.ArchivedCount
	for _, group := range page.Groups {
		response.Groups = append(response.Groups, ToGroupDTO(group))
	}
	return response, nil
}

func ToStudentDTO(student db.TeacherStudentView) StudentListItemDTO {
	item := StudentListItemDTO{
		ID:       student.ID,
		Name:     student.Name,
		Email:    student.Email,
		Grade:    student.Grade,
		State:    student.Status,
		Initials: avatarLetter(student.Name),
		Groups:   make([]StudentGroupDTO, 0, len(student.Groups)),
	}
	for _, group := range student.Groups {
		item.Groups = append(item.Groups, StudentGroupDTO{ID: group.ID, Name: group.Name})
	}
	return item
}

func ToGroupDTO(group db.TeacherGroupView) GroupListItemDTO {
	item := GroupListItemDTO{
		ID:      group.ID,
		Name:    group.Name,
		Subject: group.Subject,
		State:   group.Status,
		Members: make([]GroupMemberDTO, 0, len(group.Members)),
	}
	for _, member := range group.Members {
		item.Members = append(item.Members, GroupMemberDTO{
			ID:       member.ID,
			Name:     member.Name,
			Email:    member.Email,
			Initials: avatarLetter(member.Name),
		})
	}
	item.MemberCount = len(item.Members)
	return item
}

// avatarLetter возвращает одну букву для аватара в существующем поле "initials".
// Функция не пытается разделять произвольное имя на фамилию, имя и отчество:
// порядок слов и составные фамилии зависят от языка и не всегда известны.
// В дальнейшем frontend может использовать стандартный аватар или отдельные поля имени.
func avatarLetter(displayName string) string {
	for _, letter := range strings.TrimSpace(displayName) {
		return strings.ToUpper(string(letter))
	}
	return ""
}
