package db

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
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
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Внутренняя функция для вычисления хэша
func generateHash(t *Task) string {
	data := fmt.Sprintf("%s:%d:%s", t.Subject, t.TaskNumber, t.ContentText)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}