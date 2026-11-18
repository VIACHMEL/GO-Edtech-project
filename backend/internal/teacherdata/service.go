package teacherdata

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"
)

type Service struct {
	Store *db.DB
}

func NewService(store *db.DB) *Service {
	return &Service{Store: store}
}

func (s *Service) GetProfile(
	ctx context.Context,
	userID int64,
) (ProfileDTO, error) {
	user, err := s.Store.GetUser(ctx, userID)

	if err != nil {
		return ProfileDTO{}, err
	}

	return ToProfileDTO(*user), nil
}

func (s *Service) GetTeacherStudents(
	ctx context.Context,
	teacherID int64,
	query string,
	status string,
) (StudentListResponse, error) {

	response := StudentListResponse{
		Students: make([]StudentListItemDTO, 0),
	}

	if status == "" {
		status = "all"
	}

	if status != "all" && status != "active" {
		return response, fmt.Errorf(
			"%w: unsupported student filter", db.ErrValidation,
		)
	}

	teacher, err := s.Store.GetUser(ctx, teacherID)
	if err != nil {
		return response, err
	}

	if teacher.Role != "teacher" || teacher.Status != "active" {
		return response, fmt.Errorf(
			"%w: active teacher required", db.ErrValidation,
		)
	}

	students, err := s.Store.ListTeacherStudentViews(
		ctx, teacherID,
	)
	if err != nil {
		return response, err
	}

	response.Total = len(students)
	response.ActiveCount = len(students)
	query = strings.ToLower(strings.TrimSpace(query))

	for _, student := range students {
		item := StudentListItemDTO{
			ID:       student.ID,
			Name:     student.Name,
			Email:    student.Email,
			Grade:    student.Grade,
			State:    student.Status,
			Initials: makeInitials(student.Name),
			Groups:   make([]StudentGroupDTO, 0),
		}

		matches := query == "" ||
			strings.Contains(strings.ToLower(student.Name), query) ||
			strings.Contains(strings.ToLower(student.Email), query)

		for _, group := range student.Groups {
			item.Groups = append(item.Groups, StudentGroupDTO{
				ID: group.ID, Name: group.Name,
			})

			if strings.Contains(strings.ToLower(group.Name), query) {
				matches = true
			}
		}

		if matches {
			response.Students = append(response.Students, item)
		}
	}

	return response, nil
}

func makeInitials(name string) string {
	words := strings.Fields(name)

	var result strings.Builder

	for i, word := range words {
		if i >= 2 {
			log.Printf("WARNING: У профиля нестандартные инициалы: %v", name)
		}

		letters := []rune(word)
		result.WriteString(strings.ToUpper(string(letters[0])))
	}

	return result.String()
}

// GetTeacherGroups returns real active/archived groups and active membership.
// Status: "all", "active", "archived" (empty means "all").
// Counts are computed BEFORE search/status filters, for Django tab labels.
func (s *Service) GetTeacherGroups(
	ctx context.Context,
	teacherID int64,
	query string,
	status string,
) (GroupListResponse, error) {
	response := GroupListResponse{Groups: make([]GroupListItemDTO, 0)}

	if status == "" {
		status = "all"
	}
	if status != "all" && status != "active" && status != "archived" {
		return response, fmt.Errorf("%w: unsupported group filter", db.ErrValidation)
	}

	teacher, err := s.Store.GetUser(ctx, teacherID)
	if err != nil {
		return response, err
	}
	if teacher.Role != "teacher" || teacher.Status != "active" {
		return response, fmt.Errorf("%w: active teacher required", db.ErrValidation)
	}

	groups, err := s.Store.ListTeacherGroupViews(ctx, teacherID)
	if err != nil {
		return response, err
	}
	response.Total = len(groups)
	query = strings.ToLower(strings.TrimSpace(query))

	for _, group := range groups {
		switch group.Status {
		case "active":
			response.ActiveCount++
		case "archived":
			response.ArchivedCount++
		}

		if status != "all" && status != group.Status {
			continue
		}

		matches := query == "" || strings.Contains(strings.ToLower(group.Name), query)
		if group.Subject != nil && strings.Contains(strings.ToLower(*group.Subject), query) {
			matches = true
		}
		if !matches {
			continue
		}

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
				Initials: makeInitials(member.Name),
			})
		}
		item.MemberCount = len(item.Members)
		response.Groups = append(response.Groups, item)
	}

	return response, nil
}
