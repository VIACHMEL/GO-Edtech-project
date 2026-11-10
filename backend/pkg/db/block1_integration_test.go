package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// block1Fixture is isolated from the development database. All users created
// by these tests are removed in reverse foreign-key order during cleanup.
type block1Fixture struct {
	t       *testing.T
	ctx     context.Context
	db      *DB
	pool    *pgxpool.Pool
	userIDs []int64
	stamp   int64
}

func newBlock1Fixture(t *testing.T) *block1Fixture {
	t.Helper()
	dsn := os.Getenv("EDTECH_TEST_DSN")
	if dsn == "" {
		t.Skip("set EDTECH_TEST_DSN to run PostgreSQL integration tests")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := config.ConnConfig.Database
	if name != "edtech_test_db" && !strings.HasSuffix(name, "_test") {
		t.Fatalf("refusing to modify database %q: use edtech_test_db or a _test database", name)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		cancel()
		t.Fatal(err)
	}
	f := &block1Fixture{t: t, ctx: ctx, db: &DB{Pool: pool}, pool: pool, stamp: time.Now().UnixNano()}
	t.Cleanup(func() {
		// Cleanup uses its own context because the test context may have expired.
		c, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if len(f.userIDs) > 0 {
			for _, statement := range []string{
				"DELETE FROM invitations WHERE teacher_id = ANY($1)",
				"DELETE FROM group_members WHERE group_id IN (SELECT id FROM groups WHERE teacher_id = ANY($1))",
				"DELETE FROM groups WHERE teacher_id = ANY($1)",
				"DELETE FROM teacher_students WHERE teacher_id = ANY($1) OR student_id = ANY($1)",
				"DELETE FROM users WHERE id = ANY($1)",
			} {
				if _, err := pool.Exec(c, statement, f.userIDs); err != nil {
					t.Errorf("test cleanup: %v", err)
				}
			}
		}
		pool.Close()
		cancel()
	})
	return f
}

func (f *block1Fixture) newUser(role, label string) *User {
	f.t.Helper()
	user, err := f.db.CreateUser(f.ctx, NewUser{
		Email: fmt.Sprintf("block1-%s-%s-%d@example.invalid", role, label, f.stamp),
		Name:  label, Role: role, PasswordHash: "test-only-not-a-real-password-hash",
	})
	if err != nil {
		f.t.Fatal(err)
	}
	f.userIDs = append(f.userIDs, user.ID)
	return user
}

func (f *block1Fixture) invitation(teacher *User, groupID *int64, email *string, label string) (*Invitation, string) {
	f.t.Helper()
	tokenBytes := sha256.Sum256([]byte(fmt.Sprintf("test-%s-%d", label, f.stamp)))
	tokenHash := hex.EncodeToString(tokenBytes[:])
	inv, err := f.db.CreateInvitation(f.ctx, NewInvitation{
		TeacherID: teacher.ID, GroupID: groupID, InviteeEmail: email,
		TokenHash: tokenHash, ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return inv, tokenHash
}

func TestUsersSQLIntegration(t *testing.T) {
	f := newBlock1Fixture(t)
	teacher := f.newUser("teacher", "Teacher")
	student := f.newUser("student", "Student")

	got, err := f.db.GetUser(f.ctx, student.ID)
	if err != nil || got.ID != student.ID || got.Role != "student" {
		t.Fatalf("GetUser: %+v, %v", got, err)
	}
	credentials, err := f.db.GetUserByEmail(f.ctx, "  "+strings.ToUpper(student.Email)+"  ")
	if err != nil || credentials.ID != student.ID || credentials.PasswordHash == "" {
		t.Fatalf("GetUserByEmail: %+v, %v", credentials, err)
	}

	_, err = f.db.CreateUser(f.ctx, NewUser{
		Email: strings.ToUpper(student.Email), Name: "Duplicate", Role: "student", PasswordHash: "test-hash",
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate email must fail with ErrConflict, got %v", err)
	}

	grade := int16(10)
	updated, err := f.db.UpdateUserProfile(f.ctx, student.ID, "  Updated student  ", &grade)
	if err != nil || updated.Name != "Updated student" || updated.Grade == nil || *updated.Grade != 10 {
		t.Fatalf("UpdateUserProfile: %+v, %v", updated, err)
	}
	invalidGrade := int16(13)
	if _, err := f.db.UpdateUserProfile(f.ctx, student.ID, "Student", &invalidGrade); !errors.Is(err, ErrValidation) {
		t.Fatalf("invalid grade: expected ErrValidation, got %v", err)
	}
	if _, err := f.db.UpdateUserProfile(f.ctx, teacher.ID, "Teacher", &grade); !errors.Is(err, ErrNotFound) {
		t.Fatalf("teacher grade must be rejected, got %v", err)
	}

	if err := f.db.LinkTeacherStudent(f.ctx, teacher.ID, student.ID); err != nil {
		t.Fatal(err)
	}
	students, err := f.db.ListTeacherStudents(f.ctx, teacher.ID)
	if err != nil || len(students) != 1 || students[0].ID != student.ID {
		t.Fatalf("ListTeacherStudents: %+v, %v", students, err)
	}
	teachers, err := f.db.ListStudentTeachers(f.ctx, student.ID)
	if err != nil || len(teachers) != 1 || teachers[0].ID != teacher.ID {
		t.Fatalf("ListStudentTeachers: %+v, %v", teachers, err)
	}

	if err := f.db.DisableUser(f.ctx, student.ID); err != nil {
		t.Fatal(err)
	}
	disabled, err := f.db.GetUser(f.ctx, student.ID)
	if err != nil || disabled.Status != "disabled" {
		t.Fatalf("DisableUser: %+v, %v", disabled, err)
	}
	students, err = f.db.ListTeacherStudents(f.ctx, teacher.ID)
	if err != nil || len(students) != 0 {
		t.Fatalf("disabled student must disappear from active list: %+v, %v", students, err)
	}
}

func TestGroupSecuritySQLIntegration(t *testing.T) {
	f := newBlock1Fixture(t)
	teacher := f.newUser("teacher", "Teacher")
	otherTeacher := f.newUser("teacher", "Other")
	student := f.newUser("student", "Student")
	unlinked := f.newUser("student", "Unlinked")

	if _, err := f.db.CreateGroup(f.ctx, NewGroup{TeacherID: student.ID, Name: "Not allowed"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("student must not create a group: %v", err)
	}
	group, err := f.db.CreateGroup(f.ctx, NewGroup{TeacherID: teacher.ID, Name: "Test group"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.AddGroupMember(f.ctx, group.ID, student.ID); !errors.Is(err, ErrValidation) {
		t.Fatalf("student without teacher link must not join group: %v", err)
	}
	if err := f.db.LinkTeacherStudent(f.ctx, teacher.ID, student.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.db.AddGroupMember(f.ctx, group.ID, student.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.db.AddGroupMember(f.ctx, group.ID, student.ID); err != nil {
		t.Fatalf("repeat AddGroupMember should not fail: %v", err)
	}
	if err := f.db.AddGroupMember(f.ctx, group.ID, unlinked.ID); !errors.Is(err, ErrValidation) {
		t.Fatalf("unlinked student joined group: %v", err)
	}
	members, err := f.db.ListGroupMembers(f.ctx, group.ID)
	if err != nil || len(members) != 1 {
		t.Fatalf("members: %+v, %v", members, err)
	}

	wrongEmail := unlinked.Email
	_, wrongToken := f.invitation(teacher, &group.ID, &wrongEmail, "wrong-email")
	if err := f.db.AcceptInvitation(f.ctx, wrongToken, student.ID); !errors.Is(err, ErrValidation) {
		t.Fatalf("invite for another email must fail: %v", err)
	}

	revoked, revokedToken := f.invitation(teacher, &group.ID, nil, "revoked")
	if err := f.db.RevokeInvitation(f.ctx, revoked.ID, teacher.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.db.AcceptInvitation(f.ctx, revokedToken, student.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("revoked invite must fail: %v", err)
	}

	expired, expiredToken := f.invitation(teacher, &group.ID, nil, "expired")
	if _, err := f.pool.Exec(f.ctx, "UPDATE invitations SET expires_at = NOW() - interval '1 minute' WHERE id = $1", expired.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.db.AcceptInvitation(f.ctx, expiredToken, student.ID); !errors.Is(err, ErrValidation) {
		t.Fatalf("expired invite must fail: %v", err)
	}

	if err := f.db.UnlinkTeacherStudent(f.ctx, teacher.ID, student.ID); err != nil {
		t.Fatal(err)
	}
	members, err = f.db.ListGroupMembers(f.ctx, group.ID)
	if err != nil || len(members) != 0 {
		t.Fatalf("unlink should remove memberships: %+v, %v", members, err)
	}
	if err := f.db.LinkTeacherStudent(f.ctx, teacher.ID, student.ID); err != nil {
		t.Fatal(err)
	}
	members, err = f.db.ListGroupMembers(f.ctx, group.ID)
	if err != nil || len(members) != 0 {
		t.Fatalf("relink must not restore old group membership: %+v, %v", members, err)
	}

	if err := f.db.ArchiveGroup(f.ctx, group.ID, teacher.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.db.AddGroupMember(f.ctx, group.ID, student.ID); !errors.Is(err, ErrValidation) {
		t.Fatalf("archived group must reject new member: %v", err)
	}
	groups, err := f.db.ListTeacherGroups(f.ctx, teacher.ID)
	if err != nil || len(groups) != 0 {
		t.Fatalf("archived group should not be active: %+v, %v", groups, err)
	}

	otherGroup, err := f.db.CreateGroup(f.ctx, NewGroup{TeacherID: otherTeacher.ID, Name: "Other group"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.LinkTeacherStudent(f.ctx, otherTeacher.ID, student.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.db.AddGroupMember(f.ctx, otherGroup.ID, student.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.db.DisableUser(f.ctx, otherTeacher.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.db.AddGroupMember(f.ctx, otherGroup.ID, student.ID); !errors.Is(err, ErrValidation) {
		t.Fatalf("disabled teacher group must reject new member: %v", err)
	}
	members, err = f.db.ListGroupMembers(f.ctx, otherGroup.ID)
	if err != nil || len(members) != 0 {
		t.Fatalf("disabled teacher group must hide members: %+v, %v", members, err)
	}
}

// Checks the SELECT ... FOR UPDATE protection against two simultaneous
// requests trying to consume the same single-use invitation.
func TestConcurrentInvitationSQLIntegration(t *testing.T) {
	f := newBlock1Fixture(t)
	teacher := f.newUser("teacher", "Teacher")
	student := f.newUser("student", "Student")
	_, hash := f.invitation(teacher, nil, nil, "concurrent")

	results := make(chan error, 2)
	go func() { results <- f.db.AcceptInvitation(f.ctx, hash, student.ID) }()
	go func() { results <- f.db.AcceptInvitation(f.ctx, hash, student.ID) }()
	first, second := <-results, <-results

	accepted, conflicts := 0, 0
	for _, err := range []error{first, second} {
		if err == nil {
			accepted++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatalf("unexpected parallel accept error: %v", err)
		}
	}
	if accepted != 1 || conflicts != 1 {
		t.Fatalf("expected exactly one success and one ErrConflict; got %v, %v", first, second)
	}
	students, err := f.db.ListTeacherStudents(f.ctx, teacher.ID)
	if err != nil || len(students) != 1 || students[0].ID != student.ID {
		t.Fatalf("concurrent acceptance must create one link: %+v, %v", students, err)
	}
}
