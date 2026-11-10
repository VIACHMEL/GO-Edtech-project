package db

import "time"

// Group belongs to a teacher. Archived groups remain in the database.
type Group struct {
	ID        int64
	TeacherID int64
	Name      string
	Subject   *string
	Status    string
	CreatedAt time.Time
}

// NewGroup is the input passed by the authenticated server.
type NewGroup struct {
	TeacherID int64
	Name      string
	Subject   *string
}

// NewInvitation contains only a token hash, never a raw invitation token.
type NewInvitation struct {
	TeacherID    int64
	GroupID      *int64
	InviteeEmail *string
	TokenHash    string
	ExpiresAt    time.Time
}

// Invitation deliberately omits the secret token hash.
type Invitation struct {
	ID           int64
	TeacherID    int64
	GroupID      *int64
	InviteeEmail *string
	Status       string
	ExpiresAt    time.Time
	AcceptedBy   *int64
	AcceptedAt   *time.Time
	CreatedAt    time.Time
}
