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

// Use a SEPARATE database named edtech_test_db (or ending in _test).
// Requires tables users, teacher_students, groups, group_members, invitations.
func TestGroupInvitationIntegration(t *testing.T) {
	dsn := os.Getenv("EDTECH_TEST_DSN")
	if dsn == "" {
		t.Skip("EDTECH_TEST_DSN not set: integration test skipped")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConnConfig.Database != "edtech_test_db" && !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("refusing to run: use edtech_test_db or a database ending with _test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	db := &DB{Pool: pool}

	stamp := time.Now().UnixNano()
	teacher, err := db.CreateUser(ctx, NewUser{
		Email: fmt.Sprintf("teacher-%d@example.invalid", stamp),
		Name:  "Teacher", Role: "teacher", PasswordHash: "test-only-not-a-real-password-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	student, err := db.CreateUser(ctx, NewUser{
		Email: fmt.Sprintf("student-%d@example.invalid", stamp),
		Name:  "Student", Role: "student", PasswordHash: "test-only-not-a-real-password-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	var groupID int64
	t.Cleanup(func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanCancel()
		// Cleanup is allowed only in a separate test database.
		_, _ = pool.Exec(cleanCtx, "DELETE FROM invitations WHERE teacher_id=$1", teacher.ID)
		if groupID != 0 {
			_, _ = pool.Exec(cleanCtx, "DELETE FROM group_members WHERE group_id=$1", groupID)
			_, _ = pool.Exec(cleanCtx, "DELETE FROM groups WHERE id=$1", groupID)
		}
		_, _ = pool.Exec(cleanCtx, "DELETE FROM teacher_students WHERE teacher_id=$1", teacher.ID)
		_, _ = pool.Exec(cleanCtx, "DELETE FROM users WHERE id=$1 OR id=$2", teacher.ID, student.ID)
	})

	group, err := db.CreateGroup(ctx, NewGroup{TeacherID: teacher.ID, Name: "Test group"})
	if err != nil {
		t.Fatal(err)
	}
	groupID = group.ID
	if err := db.LinkTeacherStudent(ctx, teacher.ID, student.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.AddGroupMember(ctx, group.ID, student.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.AddGroupMember(ctx, group.ID, student.ID); err != nil {
		t.Fatalf("adding member twice must be harmless: %v", err)
	}
	members, err := db.ListGroupMembers(ctx, group.ID)
	if err != nil || len(members) != 1 || members[0].ID != student.ID {
		t.Fatalf("unexpected group members: %+v, %v", members, err)
	}
	if err := db.RemoveGroupMember(ctx, group.ID, student.ID); err != nil {
		t.Fatal(err)
	}

	raw := sha256.Sum256([]byte(fmt.Sprintf("test-random-seed-%d", stamp)))
	hash := hex.EncodeToString(raw[:])
	invite, err := db.CreateInvitation(ctx, NewInvitation{
		TeacherID: teacher.ID, GroupID: &group.ID, InviteeEmail: &student.Email,
		TokenHash: hash, ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if invite.Status != "pending" {
		t.Fatalf("unexpected invitation status: %s", invite.Status)
	}
	if err := db.AcceptInvitation(ctx, hash, student.ID); err != nil {
		t.Fatal(err)
	}
	accepted, err := db.GetInvitationByTokenHash(ctx, hash)
	if err != nil || accepted.Status != "accepted" || accepted.AcceptedBy == nil || *accepted.AcceptedBy != student.ID {
		t.Fatalf("unexpected accepted invitation: %+v, %v", accepted, err)
	}
	members, err = db.ListGroupMembers(ctx, group.ID)
	if err != nil || len(members) != 1 {
		t.Fatalf("invitation did not add member: %+v, %v", members, err)
	}
	if err := db.AcceptInvitation(ctx, hash, student.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("second acceptance must fail with ErrConflict, got %v", err)
	}
}
