package db

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
	"errors"
)

var (
	ErrValidation = errors.New("db: validation failed")
	ErrNotFound   = errors.New("db: record not found")
)

const (
    ArtifactTypeImage      = "image"
    ArtifactTypeSVG        = "svg"
    ArtifactTypeTikZ       = "tikz"
    ArtifactTypeAttachment = "attachment"
    ArtifactTypePreview    = "preview"

    StorageLocal    = "local"
    StorageS3       = "s3"
    StorageExternal = "external"

    StatusPending   = "pending"
    StatusDone      = "done"
    StatusFailed    = "failed"
    StatusAbandoned = "abandoned"
    StatusExternal  = "external"
)

type Task struct {
	// ЗАПОЛНЯЕТ ПАРСЕР
	SourceURL   string
	SourceSite  string
	Subject     string
	TaskNumber  int
	ContentText string
	Answer      string

	Year        int
	ExamType    string
	TaskType    string
	Solution    string
	
	Metadata    map[string]any

	// ЗАПОЛНЯЕТ БАЗА
	ID          int64
	ContentHash string
	CurrentVersionID int64
	VersionNumber    int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Внутренняя функция для вычисления хэша
func generateHash(t *Task) string {
	data := fmt.Sprintf("%s:%d:%s", t.Subject, t.TaskNumber, t.ContentText)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

type TaskArtifact struct {
    ID               int64
    TaskVersionID    int64
    Type             string
    ArtifactKey      string
    FilePath         string   // "" если storage_type == "external"
    ExternalURL      string   // "" если файл наш
    OriginalFilename string
    MimeType         string
    FileSize         int64
    Checksum         string   // "sha256:..."
    StorageType      string   // "local" | "s3" | "external"
    Status           string
	ErrorMessage     string
    CreatedAt        time.Time
    UpdatedAt        time.Time
}

type AddTaskResult struct {
    TaskID       int64
    VersionID    int64
    VersionNum   int
    WasCreated   bool
}

type TaskFilter struct {
	Subject    string
	Year       int
	TaskNumber int
	TaskType   string
	
	Limit      int    // Кол-во возвращаемых задач
}