package teacherdata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Интеграционный тест работает только с отдельной тестовой базой PostgreSQL.
// Нужные таблицы: users, teacher_students, groups, group_members.
func TestTeacherDataIntegration(t *testing.T) {
	dsn := os.Getenv("EDTECH_TEST_DSN")
	if dsn == "" {
		t.Skip("не задана переменная EDTECH_TEST_DSN")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	dbName := cfg.ConnConfig.Database
	if dbName != "edtech_test_db" && !strings.HasSuffix(dbName, "_test") {
		t.Fatalf("запрещено использовать нетестовую базу %q", dbName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := db.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}

	ids := make([]int64, 0, 3)
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		defer store.Close()
		for _, sql := range []string{
			"DELETE FROM invitations WHERE teacher_id = ANY($1)",
			"DELETE FROM group_members WHERE group_id IN (SELECT id FROM groups WHERE teacher_id = ANY($1))",
			"DELETE FROM groups WHERE teacher_id = ANY($1)",
			"DELETE FROM teacher_students WHERE teacher_id = ANY($1) OR student_id = ANY($1)",
			"DELETE FROM users WHERE id = ANY($1)",
		} {
			if _, err := store.Pool.Exec(cleanupCtx, sql, ids); err != nil {
				t.Errorf("не удалось удалить тестовые данные: %v", err)
			}
		}
	})

	stamp := time.Now().UnixNano()
	makeUser := func(role, label string) *db.User {
		t.Helper()
		user, err := store.CreateUser(ctx, db.NewUser{
			Email:        fmt.Sprintf("teacherdata-%d-%d@example.invalid", stamp, len(ids)+1),
			Name:         label,
			Role:         role,
			PasswordHash: "test-only-not-a-real-password-hash",
		})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, user.ID)
		return user
	}
	teacher := makeUser("teacher", "Первый Преподаватель")
	secondTeacher := makeUser("teacher", "Второй Преподаватель")
	student := makeUser("student", "Иван Петров")
	subject := "Информатика"

	if err := store.LinkTeacherStudent(ctx, teacher.ID, student.ID); err != nil {
		t.Fatal(err)
	}
	active, err := store.CreateGroup(ctx, db.NewGroup{TeacherID: teacher.ID, Name: "ЕГЭ 2027", Subject: &subject})
	if err != nil {
		t.Fatal(err)
	}
	archived, err := store.CreateGroup(ctx, db.NewGroup{TeacherID: teacher.ID, Name: "Прошлогодняя группа"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateGroup(ctx, db.NewGroup{TeacherID: teacher.ID, Name: "Пустая группа"}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddGroupMember(ctx, active.ID, student.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.AddGroupMember(ctx, archived.ID, student.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.ArchiveGroup(ctx, archived.ID, teacher.ID); err != nil {
		t.Fatal(err)
	}

	// Ученик может заниматься у нескольких преподавателей, но данные групп
	// другого преподавателя не должны попадать в текущую выборку.
	if err := store.LinkTeacherStudent(ctx, secondTeacher.ID, student.ID); err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateGroup(ctx, db.NewGroup{TeacherID: secondTeacher.ID, Name: "Чужая группа"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddGroupMember(ctx, other.ID, student.ID); err != nil {
		t.Fatal(err)
	}

	svc := NewService(store)
	profile, err := svc.GetProfile(ctx, student.ID)
	if err != nil || profile.ID != student.ID || profile.Name != student.Name {
		t.Fatalf("неожиданный профиль: %+v, %v", profile, err)
	}
	encoded, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(encoded)), "password") {
		t.Fatalf("обнаружено поле пароля: %s", encoded)
	}

	students, err := svc.GetTeacherStudents(ctx, teacher.ID, "", "all")
	if err != nil || students.Total != 1 || len(students.Students) != 1 || students.Students[0].ID != student.ID {
		t.Fatalf("неожиданный список учеников: %+v, %v", students, err)
	}
	if len(students.Students[0].Groups) != 1 || students.Students[0].Groups[0].ID != active.ID {
		t.Fatalf("в список ученика попала архивная или чужая группа: %+v", students.Students[0].Groups)
	}
	filteredStudents, err := svc.GetTeacherStudents(ctx, teacher.ID, "егэ", "active")
	if err != nil || len(filteredStudents.Students) != 1 {
		t.Fatalf("ошибка поиска учеников: %+v, %v", filteredStudents, err)
	}
	if _, err := svc.GetTeacherStudents(ctx, teacher.ID, "", "attention"); !errors.Is(err, db.ErrValidation) {
		t.Fatalf("фильтр attention должен возвращать ошибку: %v", err)
	}

	groups, err := svc.GetTeacherGroups(ctx, teacher.ID, "", "all")
	if err != nil || groups.Total != 3 || groups.ActiveCount != 2 || groups.ArchivedCount != 1 || len(groups.Groups) != 3 {
		t.Fatalf("неожиданные счётчики групп: %+v, %v", groups, err)
	}
	counts := make(map[int64]int)
	for _, g := range groups.Groups {
		counts[g.ID] = g.MemberCount
	}
	if counts[active.ID] != 1 || counts[archived.ID] != 1 {
		t.Fatalf("неверное число участников, включая архив: %+v", counts)
	}
	activeGroups, err := svc.GetTeacherGroups(ctx, teacher.ID, "", "active")
	if err != nil || len(activeGroups.Groups) != 2 {
		t.Fatalf("ошибка фильтра активных групп: %+v, %v", activeGroups, err)
	}
	archivedGroups, err := svc.GetTeacherGroups(ctx, teacher.ID, "", "archived")
	if err != nil || len(archivedGroups.Groups) != 1 || archivedGroups.Groups[0].ID != archived.ID {
		t.Fatalf("ошибка фильтра архивных групп: %+v, %v", archivedGroups, err)
	}
	searched, err := svc.GetTeacherGroups(ctx, teacher.ID, "информат", "all")
	if err != nil || len(searched.Groups) != 1 || searched.Groups[0].ID != active.ID {
		t.Fatalf("ошибка поиска групп по предмету: %+v, %v", searched, err)
	}
	second, err := svc.GetTeacherGroups(ctx, secondTeacher.ID, "", "all")
	if err != nil || second.Total != 1 || second.Groups[0].ID != other.ID {
		t.Fatalf("ошибка разграничения доступа преподавателей: %+v, %v", second, err)
	}
	if _, err := svc.GetTeacherStudents(ctx, teacher.ID, "", "paused"); !errors.Is(err, db.ErrValidation) {
		t.Fatalf("статус paused отсутствует в модели учеников: %v", err)
	}
	alias, err := svc.GetTeacherStudents(ctx, teacher.ID, "", "active")
	if err != nil || alias.Total != students.Total || alias.ActiveCount != alias.Total {
		t.Fatalf("all и active должны возвращать одни и те же действующие связи: %+v, %v", alias, err)
	}
	if err := store.UnlinkTeacherStudent(ctx, teacher.ID, student.ID); err != nil {
		t.Fatal(err)
	}
	afterUnlink, err := svc.GetTeacherStudents(ctx, teacher.ID, "", "all")
	if err != nil || afterUnlink.Total != 0 || len(afterUnlink.Students) != 0 {
		t.Fatalf("прекращённая связь не должна отображаться: %+v, %v", afterUnlink, err)
	}
	if _, err := svc.GetTeacherGroups(ctx, student.ID, "", "all"); !errors.Is(err, db.ErrValidation) {
		t.Fatalf("ученик не должен получать группы преподавателя: %v", err)
	}
}
