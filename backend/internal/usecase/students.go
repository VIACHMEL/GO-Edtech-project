// Package usecase связывает обработчики с хранилищем.
package usecase

import (
	"context"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/storage"
)

type StudentStore interface {
	Students(context.Context) ([]storage.Student, error)
}

type Service struct{ students StudentStore }

func New(students StudentStore) *Service { return &Service{students: students} }

func (s *Service) DemoStudents(ctx context.Context) ([]storage.Student, error) {
	return s.students.Students(ctx)
}
