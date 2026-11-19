package db

import (
	"context"
	"fmt"
	"strings"
)

// TeacherStudentsPage — модель чтения на уровне БД, а не HTTP/JSON-ответ.
// В список попадают только активные ученики с действующей связью с преподавателем.
// Состояния «обучение приостановлено» в текущей схеме БД нет.
type TeacherStudentsPage struct {
	Students []TeacherStudentView
	Total    int
}

// TeacherGroupsPage — модель чтения групп со счётчиками до применения фильтров.
type TeacherGroupsPage struct {
	Groups        []TeacherGroupView
	Total         int
	ActiveCount   int
	ArchivedCount int
}

// requireActiveTeacher проверяет роль и статус пользователя на уровне БД.
// HTTP-обработчик обязан брать teacherID из подтверждённой сессии, а не из запроса клиента.
func (db *DB) requireActiveTeacher(ctx context.Context, teacherID int64) error {
	teacher, err := db.GetUser(ctx, teacherID)
	if err != nil {
		return err
	}
	if teacher.Role != "teacher" || teacher.Status != "active" {
		return fmt.Errorf("%w: требуется активный преподаватель", ErrValidation)
	}
	return nil
}

// ListTeacherStudentsPage возвращает учеников с действующей учебной связью.
// Фильтры "all" и "active" равнозначны и сохранены для совместимости: удалённые
// связи и заблокированные аккаунты не отображаются.
// Пока поиск выполняется в памяти после SQL-запроса с JOIN. При росте числа
// учеников следует добавить фильтрацию и постраничную выдачу средствами SQL.
func (db *DB) ListTeacherStudentsPage(ctx context.Context, teacherID int64, query, status string) (TeacherStudentsPage, error) {
	page := TeacherStudentsPage{Students: make([]TeacherStudentView, 0)}
	if status != "" && status != "all" && status != "active" {
		return page, fmt.Errorf("%w: неподдерживаемый фильтр учеников", ErrValidation)
	}
	if err := db.requireActiveTeacher(ctx, teacherID); err != nil {
		return page, err
	}
	students, err := db.ListTeacherStudentViews(ctx, teacherID)
	if err != nil {
		return page, err
	}
	page.Total = len(students)
	q := strings.ToLower(strings.TrimSpace(query))
	for _, student := range students {
		if matchesTeacherStudent(student, q) {
			page.Students = append(page.Students, student)
		}
	}
	return page, nil
}

func matchesTeacherStudent(student TeacherStudentView, query string) bool {
	if query == "" || strings.Contains(strings.ToLower(student.Name), query) ||
		strings.Contains(strings.ToLower(student.Email), query) {
		return true
	}
	for _, group := range student.Groups {
		if strings.Contains(strings.ToLower(group.Name), query) {
			return true
		}
	}
	return false
}

// ListTeacherGroupsPage возвращает действующие и архивные группы преподавателя.
// Счётчики рассчитываются по полному списку до поиска и фильтра по статусу.
func (db *DB) ListTeacherGroupsPage(ctx context.Context, teacherID int64, query, status string) (TeacherGroupsPage, error) {
	page := TeacherGroupsPage{Groups: make([]TeacherGroupView, 0)}
	if status == "" {
		status = "all"
	}
	if status != "all" && status != "active" && status != "archived" {
		return page, fmt.Errorf("%w: неподдерживаемый фильтр групп", ErrValidation)
	}
	if err := db.requireActiveTeacher(ctx, teacherID); err != nil {
		return page, err
	}
	groups, err := db.ListTeacherGroupViews(ctx, teacherID)
	if err != nil {
		return page, err
	}
	page.Total = len(groups)
	q := strings.ToLower(strings.TrimSpace(query))
	for _, group := range groups {
		switch group.Status {
		case "active":
			page.ActiveCount++
		case "archived":
			page.ArchivedCount++
		}
		if status != "all" && group.Status != status {
			continue
		}
		if matchesTeacherGroup(group, q) {
			page.Groups = append(page.Groups, group)
		}
	}
	return page, nil
}

func matchesTeacherGroup(group TeacherGroupView, query string) bool {
	if query == "" || strings.Contains(strings.ToLower(group.Name), query) {
		return true
	}
	return group.Subject != nil && strings.Contains(strings.ToLower(*group.Subject), query)
}
