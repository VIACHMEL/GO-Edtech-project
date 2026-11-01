package db

import (
	"context"
	"fmt"
	"strings"
	"errors"

	"github.com/jackc/pgx/v5"
)

const insertTaskQuery = `
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

const selectTaskColumns = `
	id, source_url, source_site, subject, task_number, content_text, answer,
	year, exam_type, task_type, solution, metadata, content_hash, created_at, updated_at
`
const DEFAULT_TASK_LIMIT = 20

//Проверяет наличие обязательных полей и генерирует хеш, возрващает ошибку
func validateAndPrepare(t *Task) error {
	t.SourceURL = strings.TrimSpace(t.SourceURL)
	t.SourceSite = strings.TrimSpace(t.SourceSite)
	t.Subject = strings.TrimSpace(t.Subject)
	t.ContentText = strings.TrimSpace(t.ContentText)
	t.Answer = strings.TrimSpace(t.Answer)
	t.Solution = strings.TrimSpace(t.Solution)

	if t.SourceURL == "" {
		return fmt.Errorf("%w: source_url is required", ErrValidation)
	}
	if t.Subject == "" {
		return fmt.Errorf("%w: subject is required", ErrValidation)
	}
	if t.ContentText == "" {
		return fmt.Errorf("%w: content_text is required", ErrValidation)
	}

	if t.TaskNumber < 0 {
		return fmt.Errorf("%w: task_number cannot be negative", ErrValidation)
	}

	if t.Metadata == nil {
		t.Metadata = make(map[string]any)
	}

	t.ContentHash = generateHash(t)
	return nil
}	

//Проделывает валидацию для пачки задач
func validateAndPrepareAllTasks(tasks []*Task) error {
	for _, t := range tasks {
		if err := validateAndPrepare(t); err != nil {
			return fmt.Errorf("validation failed for task %s: %w", t.SourceURL, err)
		}
	}
	return nil
}

func buildTaskArgs(t *Task) []any {
	return []any{
		t.SourceURL, t.SourceSite, t.Subject, t.TaskNumber, t.ContentText, t.Answer,
		t.Year, t.ExamType, t.TaskType, t.Solution, t.Metadata, t.ContentHash,
	}
}

func buildBatch(tasks []*Task) *pgx.Batch {
	batch := &pgx.Batch{}
	for _, t := range tasks {
		batch.Queue(insertTaskQuery, buildTaskArgs(t)...)
	}
	return batch
}

// Отправляет пакет заданий в БД
func executeBatch(ctx context.Context, transaction pgx.Tx, batch *pgx.Batch, count int) error {
	batchResults := transaction.SendBatch(ctx, batch)
	defer batchResults.Close()

	for i := 0; i < count; i++ {
		_, err := batchResults.Exec()
		if err != nil {
			return fmt.Errorf("failed to execute batch insert: %w", err)
		}
	}
	return nil
}

// Сканирует БД по строке и заполняет структуру Task
func scanTask(row pgx.Row) (*Task, error) {
	var t Task
	err := row.Scan(
		&t.ID, &t.SourceURL, &t.SourceSite, &t.Subject, &t.TaskNumber, &t.ContentText, &t.Answer,
		&t.Year, &t.ExamType, &t.TaskType, &t.Solution, &t.Metadata, &t.ContentHash, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func rowsToTasks(rows pgx.Rows) ([]*Task, error) {
	var tasks []*Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan task: %w", err)
		}
		tasks = append(tasks, task)
	}

	// Проверяем, не было ли ошибки во время итерации
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during rows iteration: %w", err)
	}

	return tasks, nil
}

// Добавляет задание и возвращает ID
func (db *DB) AddTask(ctx context.Context, t *Task) (int64, error) {
	if err := validateAndPrepare(t); err != nil {
		return 0, err
	}
	
	var id int64
	err := db.Pool.QueryRow(ctx, insertTaskQuery, buildTaskArgs(t)...).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to add task: %w", err)
	}

	return id, nil
}

// сохраняет пачку заданий в одной транзакции
func (db *DB) AddTasks(ctx context.Context, tasks []*Task) error {
	if len(tasks) == 0 {
		return nil
	}

	if err := validateAndPrepareAllTasks(tasks); err != nil {
		return err
	}

	// Начинаем транзакцию
	transaction, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer transaction.Rollback(ctx)

	batch := buildBatch(tasks)
	if err := executeBatch(ctx, transaction, batch, len(tasks)); err != nil {
		return err
	}

	// Фиксируем транзакцию
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// получить задачу по ID
func (db *DB) GetTask(ctx context.Context, id int64) (*Task, error) {
	query := `SELECT ` + selectTaskColumns + ` FROM tasks WHERE id = $1`

	task, err := scanTask(db.Pool.QueryRow(ctx, query, id))
	
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}
	
	return task, nil
}

// получить набор задач по ID
func (db *DB) GetTasksByIDs(ctx context.Context, ids []int64) ([]*Task, error) {
	if len(ids) == 0 {
		return []*Task{}, nil
	}

	query := `SELECT ` + selectTaskColumns + ` FROM tasks WHERE id = ANY($1)`
	
	rows, err := db.Pool.Query(ctx, query, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get tasks by ids: %w", err)
	}
	defer rows.Close()

	return rowsToTasks(rows)
}

// Возвращает случайные задания, соответствующие фильтру
func (db *DB) ListTasks(ctx context.Context, filter TaskFilter) ([]*Task, error) {
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 20 // Дефолтный лимит, если фронтенд не передал
	}
	
	query := `
		SELECT ` + selectTaskColumns + `
		FROM tasks
		WHERE 
			($1 = '' OR subject = $1)
			AND ($2 = 0 OR year = $2)
			AND ($3 = 0 OR task_number = $3)
			AND ($4 = '' OR task_type = $4)
		ORDER BY RANDOM() 
		LIMIT $5
	`

	rows, err := db.Pool.Query(ctx, query, 
		filter.Subject, 
		filter.Year, 
		filter.TaskNumber, 
		filter.TaskType, 
		filter.Limit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", err)
	}
	defer rows.Close()

	return rowsToTasks(rows)
}