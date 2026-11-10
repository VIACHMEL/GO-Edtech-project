package db

import (
	"errors"
	"testing"
)

func TestNormalizeUserEmail(t *testing.T) {
	got, err := normalizeUserEmail("  STUDENT@Example.com ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "student@example.com" {
		t.Fatalf("got %q", got)
	}
}

func TestValidateNewUser(t *testing.T) {
	studentGrade := int16(11)
	input, err := validateNewUser(NewUser{
		Email: "STUDENT@example.com", PasswordHash: "hashed-not-real-password",
		Name: " Test ", Role: "student", Grade: &studentGrade,
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.Email != "student@example.com" || input.Name != "Test" {
		t.Fatalf("normalization did not work: %+v", input)
	}
	_, err = validateNewUser(NewUser{Email: "teacher@example.com", PasswordHash: "hash", Name: "T", Role: "teacher", Grade: &studentGrade})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}
