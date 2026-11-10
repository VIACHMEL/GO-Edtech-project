package db

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestValidateGroupInput(t *testing.T) {
	subject := "   информатика  "
	name, gotSubject, err := validateGroupInput("  ЕГЭ-2027  ", &subject)
	if err != nil || name != "ЕГЭ-2027" || gotSubject == nil || *gotSubject != "информатика" {
		t.Fatalf("unexpected validation result: %q, %v, %v", name, gotSubject, err)
	}
	_, _, err = validateGroupInput("  ", nil)
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestValidateInvitation(t *testing.T) {
	valid := NewInvitation{TeacherID: 1, TokenHash: strings.Repeat("a", 64), ExpiresAt: time.Now().Add(time.Hour)}
	_, err := validateNewInvitation(valid)
	if err != nil {
		t.Fatal(err)
	}
	valid.TokenHash = "secret-token"
	_, err = validateNewInvitation(valid)
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}
