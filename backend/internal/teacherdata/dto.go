package teacherdata

import (
	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"
)

type ProfileDTO struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	Status string `json:"status"`
}

type StudentGroupDTO struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type StudentListItemDTO struct {
	ID       int64             `json:"id"`
	Name     string            `json:"name"`
	Email    string            `json:"email"`
	Grade    *int16            `json:"grade"`
	State    string            `json:"state"`
	Initials string            `json:"initials"`
	Groups   []StudentGroupDTO `json:"groups"`
}

type StudentListResponse struct {
	Students    []StudentListItemDTO `json:"students"`
	Total       int                  `json:"total"`
	ActiveCount int                  `json:"active_count"`
}

// Преобразует профиль из БД в структуру для frontend.
func ToProfileDTO(user db.User) ProfileDTO {
	return ProfileDTO{
		ID:     user.ID,
		Name:   user.Name,
		Email:  user.Email,
		Role:   user.Role,
		Status: user.Status,
	}
}

// GroupMemberDTO is the public part of a student profile used inside a group.
type GroupMemberDTO struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Initials string `json:"initials"`
}

// GroupListItemDTO contains only fields supported by the current DB schema.
type GroupListItemDTO struct {
	ID          int64            `json:"id"`
	Name        string           `json:"name"`
	Subject     *string          `json:"subject"`
	State       string           `json:"state"`
	MemberCount int              `json:"member_count"`
	Members     []GroupMemberDTO `json:"members"`
}

type GroupListResponse struct {
	Groups        []GroupListItemDTO `json:"groups"`
	Total         int                `json:"total"`
	ActiveCount   int                `json:"active_count"`
	ArchivedCount int                `json:"archived_count"`
}
