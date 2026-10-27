package db

import (
	"context"
	"fmt"
)

// AddTask сохраняет задание и возвращает его ID
func (db *DB) AddTask(ctx context.Context, t *Task) (int64, error) {
	t.ContentHash = generateHash(t)

	query := `
		INSERT INTO tasks (
			source_url, source_site, subject, task_number, content_text, answer,
			year, exam_type, task_type, solution, metadata, content_hash
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (source_url, content_hash) 
		DO UPDATE SET 
			answer = EXCLUDED.answer,
			solution = EXCLUDED.solution,
			metadata = EXCLUDED.metadata,
			updated_at = now()
		RETURNING id;
	`

	var id int64
	err := db.Pool.QueryRow(ctx, query,
		t.SourceURL, t.SourceSite, t.Subject, t.TaskNumber, t.ContentText, t.Answer,
		t.Year, t.ExamType, t.TaskType, t.Solution, t.Metadata, t.ContentHash,
	).Scan(&id)

	if err != nil {
		return 0, fmt.Errorf("failed to add task: %w", err)
	}

	return id, nil
}