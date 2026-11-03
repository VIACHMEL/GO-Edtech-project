package db

import (
	"context"
	"fmt"
	"strings"
	"errors"
	"regexp"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const upsertTaskQuery = `
	WITH upserted_task AS (
        INSERT INTO tasks (source_url, source_site, subject, task_number,
                           year, exam_type, task_type, metadata)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
        ON CONFLICT (source_url) DO UPDATE SET
            metadata = EXCLUDED.metadata,
            updated_at = now()
        RETURNING id
    ),
    inserted_version AS (
        INSERT INTO task_versions
            (task_id, content_text, answer, solution, content_hash, version_number)
        SELECT
            ut.id, $9, $10, $11, $12,
            COALESCE((SELECT MAX(version_number) FROM task_versions
                      WHERE task_id = ut.id), 0) + 1
        FROM upserted_task ut
        ON CONFLICT (task_id, content_hash) DO UPDATE SET
            answer = EXCLUDED.answer,
            solution = EXCLUDED.solution
        RETURNING task_id, id, version_number, (xmax = 0) AS was_created
    )
    SELECT task_id, id, version_number, was_created FROM inserted_version;
`

const selectTaskColumns = `
	t.id, t.source_url, t.source_site, t.subject, t.task_number,
	t.year, t.exam_type, t.task_type, t.metadata, t.current_version_id,
	t.created_at, t.updated_at,
	v.content_text, v.answer, v.solution, v.content_hash, v.version_number
`
const defaultTaskLimit = 20
const minFilterLimit = 0
const maxFilterLimit = 100

func validateTask(t *Task) error {
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
	return nil
}

// подводит текст к единому виду и генерирует хеш
func prepareTask(t *Task) {
	t.SourceURL = strings.TrimSpace(t.SourceURL)
	t.SourceSite = strings.TrimSpace(t.SourceSite)
	t.Subject = strings.TrimSpace(t.Subject)
	t.ContentText = strings.TrimSpace(t.ContentText)
	t.Answer = strings.TrimSpace(t.Answer)
	t.Solution = strings.TrimSpace(t.Solution)

	if t.Metadata == nil {
		t.Metadata = make(map[string]any)
	}

	t.ContentHash = generateHash(t)
}	

//Проделывает валидацию для пачки задач
func validateAndPrepareAllTasks(tasks []*Task) error {
    for _, t := range tasks {
        prepareTask(t)                           
        if err := validateTask(t); err != nil {
            return fmt.Errorf("validation failed for task %s: %w", t.SourceURL, err)
        }
    }
    return nil
}

func buildTaskArgs(t *Task) []any {
	return []any{
		// Для таблицы tasks ($1 - $8)
		t.SourceURL, t.SourceSite, t.Subject, t.TaskNumber,
		t.Year, t.ExamType, t.TaskType, t.Metadata,
		// Для таблицы task_versions ($9 - $12)
		t.ContentText, t.Answer, t.Solution, t.ContentHash,
	}
}

func buildBatch(tasks []*Task) *pgx.Batch {
	batch := &pgx.Batch{}
	for _, t := range tasks {
		batch.Queue(upsertTaskQuery, buildTaskArgs(t)...)
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
		// Из таблицы tasks (1-12)
		&t.ID, &t.SourceURL, &t.SourceSite, &t.Subject, &t.TaskNumber,
		&t.Year, &t.ExamType, &t.TaskType, &t.Metadata, &t.CurrentVersionID,
		&t.CreatedAt, &t.UpdatedAt,
		// Из таблицы task_versions (13-17)
		&t.ContentText, &t.Answer, &t.Solution, &t.ContentHash, &t.VersionNumber,
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

var artifactKeyRe = regexp.MustCompile(`^[a-zA-Z0-9_\-]{1,64}$`)

func validateArtifactKey(key string) (string, error) {
    if key == "" {
        key = "main"
    }
    if !artifactKeyRe.MatchString(key) {
        return key, fmt.Errorf("%w: artifact_key must match [a-zA-Z0-9_-]{1,64}", ErrValidation)
    }
    return key, nil
}

var validArtifactTypes = map[string]bool{
    ArtifactTypeImage: true, ArtifactTypeSVG: true, ArtifactTypeTikZ: true,
    ArtifactTypeAttachment: true, ArtifactTypePreview: true,
}

func validateArtifactType(typ string) error {
    if !validArtifactTypes[typ] {
        return fmt.Errorf("%w: unknown artifact type %q", ErrValidation, typ)
    }
    return nil
}

var validArtifactStatuses = map[string]bool{
    StatusPending: true, StatusDone: true, StatusFailed: true,
    StatusAbandoned: true, StatusExternal: true,
}

func validateArtifactStatus(s string) error {
    if !validArtifactStatuses[s] {
        return fmt.Errorf("%w: unknown artifact status %q", ErrValidation, s)
    }
    return nil
}

func isUniqueViolation(err error) bool {
    var pgErr *pgconn.PgError
    return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// Добавляет задание и возвращает ID
func (db *DB) AddTask(ctx context.Context, t *Task) (*AddTaskResult, error) {
    prepareTask(t)
    if err := validateTask(t); err != nil {
        return nil, err
    }

    var res AddTaskResult
    err := db.Pool.QueryRow(ctx, upsertTaskQuery, buildTaskArgs(t)...).
        Scan(&res.TaskID, &res.VersionID, &res.VersionNum, &res.WasCreated)
    if err != nil {
        // retry на 23505 (unique_violation от параллельной вставки version_number)
        if isUniqueViolation(err) {
            for i := 0; i < 3; i++ {
                err = db.Pool.QueryRow(ctx, upsertTaskQuery, buildTaskArgs(t)...).
                    Scan(&res.TaskID, &res.VersionID, &res.VersionNum, &res.WasCreated)
                if err == nil {
                    break
                }
                if !isUniqueViolation(err) {
                    break
                }
            }
        }
        if err != nil {
            return nil, fmt.Errorf("add task: %w", err)
        }
    }
    return &res, nil
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
	query := `
		SELECT ` + selectTaskColumns + `
		FROM tasks t
		JOIN task_versions v ON t.current_version_id = v.id
		WHERE t.id = $1
	`

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

	query := `
		SELECT ` + selectTaskColumns + `
		FROM tasks t
		JOIN task_versions v ON t.current_version_id = v.id
		WHERE t.id = ANY($1)
	`
	
	rows, err := db.Pool.Query(ctx, query, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get tasks by ids: %w", err)
	}
	defer rows.Close()

	return rowsToTasks(rows)
}

// Возвращает случайные задания, соответствующие фильтру
func (db *DB) ListTasks(ctx context.Context, filter TaskFilter) ([]*Task, error) {
	if filter.Limit <= minFilterLimit || filter.Limit > maxFilterLimit {
		filter.Limit = defaultTaskLimit
	}
	
	query := `
		SELECT ` + selectTaskColumns + `
		FROM tasks t
		JOIN task_versions v ON t.current_version_id = v.id
		WHERE 
			($1 = '' OR t.subject = $1)
			AND ($2 = 0 OR t.year = $2)
			AND ($3 = 0 OR t.task_number = $3)
			AND ($4 = '' OR t.task_type = $4)
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

// Фиксирует намерение сохранить артефакт
// Вызывать ДО скачивания/рендера
func (db *DB) CreatePendingArtifact(ctx context.Context,
	versionID int64, typ, key, externalURL, originalFilename string) error {
	key, err := validateArtifactKey(key)
	if err != nil {
		return err
	}
	if err := validateArtifactType(typ); err != nil {
		return err
	}

	query := `
        INSERT INTO task_artifacts
            (task_version_id, type, artifact_key, external_url,
             original_filename, status, storage_type, error_message)
        VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), 'pending',
                CASE WHEN $4 <> '' THEN 'external' ELSE 'local' END, '')
        ON CONFLICT (task_version_id, type, artifact_key) DO UPDATE SET
            external_url      = EXCLUDED.external_url,
            original_filename = COALESCE(EXCLUDED.original_filename,
                                         task_artifacts.original_filename),
            status            = CASE
                                    WHEN task_artifacts.status = 'done'
                                     AND task_artifacts.checksum <> ''
                                    THEN 'done'
                                    ELSE 'pending'
                                END,
            error_message     = '',
            updated_at        = now()
    `
	_, err = db.Pool.Exec(ctx, query, versionID, typ, key, externalURL, originalFilename)
	if err != nil {
		return fmt.Errorf("create pending artifact: %w", err)
	}
	return nil
}

// Отмечает артефакт готовым 
// Вызывать ПОСЛЕ атомарного перемещения файла в финальное место
// Если нужно будет брать ресурс извне, отправлять в MarkExternal
func (db *DB) CompleteArtifact(ctx context.Context,
	versionID int64, typ, key, filePath, mimeType string,
	fileSize int64, checksum string) error {
	key, err := validateArtifactKey(key)
	if err != nil {
		return err
	}
	if filePath == "" {
		return fmt.Errorf("%w: file_path is required for local artifacts", ErrValidation)
	}
	if checksum == "" {
		return fmt.Errorf("%w: checksum is required", ErrValidation)
	}

	query := `
        UPDATE task_artifacts
        SET file_path     = $4,
            mime_type     = $5,
            file_size     = $6,
            checksum      = $7,
            storage_type  = 'local',
            status        = 'done',
            error_message = '',
            updated_at    = now()
        WHERE task_version_id = $1 AND type = $2 AND artifact_key = $3
    `
	tag, err := db.Pool.Exec(ctx, query, versionID, typ, key,
		filePath, mimeType, fileSize, checksum)
	if err != nil {
		return fmt.Errorf("complete artifact: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: artifact %s/%s for version %d",
			ErrNotFound, typ, key, versionID)
	}
	return nil
}
// Обновляет статус конкретного артефакта
func (db *DB) UpdateArtifactStatus(ctx context.Context,
	versionID int64, typ, key, status string) error {
	key, err := validateArtifactKey(key)
	if err != nil {
		return err
	}
	if err := validateArtifactStatus(status); err != nil {
		return err
	}

	query := `
        UPDATE task_artifacts
        SET status        = $1,
            error_message = CASE WHEN $1 = 'failed' THEN error_message ELSE '' END,
            updated_at    = now()
        WHERE task_version_id = $2 AND type = $3 AND artifact_key = $4
    `
	tag, err := db.Pool.Exec(ctx, query, status, versionID, typ, key)
	if err != nil {
		return fmt.Errorf("failed to update artifact status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: artifact %s/%s for version %d",
			ErrNotFound, typ, key, versionID)
	}
	return nil
}

// Пометить что файл хранится где-то в интернете
// Для видео/аудио/больших файлов
func (db *DB) MarkExternal(ctx context.Context,
	versionID int64, typ, key, externalURL, mimeType string) error {
	key, err := validateArtifactKey(key)
	if err != nil {
		return err
	}
	if err := validateArtifactType(typ); err != nil {
		return err
	}
	if externalURL == "" {
		return fmt.Errorf("%w: external_url is required", ErrValidation)
	}

	query := `
        UPDATE task_artifacts
        SET storage_type  = 'external',
            file_path     = NULL,
            external_url  = $4,
            mime_type     = $5,
            status        = 'external',
            error_message = '',
            updated_at    = now()
        WHERE task_version_id = $1 AND type = $2 AND artifact_key = $3
    `
	tag, err := db.Pool.Exec(ctx, query, versionID, typ, key, externalURL, mimeType)
	if err != nil {
		return fmt.Errorf("mark external: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: artifact %s/%s for version %d",
			ErrNotFound, typ, key, versionID)
	}
	return nil
}

// Вызывать при любой ошибке
func (db *DB) FailArtifact(ctx context.Context,
	versionID int64, typ, key, errMsg string) error {
	key, err := validateArtifactKey(key)
	if err != nil {
		return err
	}

	query := `
        UPDATE task_artifacts
        SET status        = 'failed',
            error_message = $4,
            updated_at    = now()
        WHERE task_version_id = $1 AND type = $2 AND artifact_key = $3
    `
	tag, err := db.Pool.Exec(ctx, query, versionID, typ, key, errMsg)
	if err != nil {
		return fmt.Errorf("fail artifact: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: artifact %s/%s for version %d",
			ErrNotFound, typ, key, versionID)
	}
	return nil
}

// Достает все артефакты для конкретной версии задачи
func (db *DB) GetArtifactsByVersionID(ctx context.Context, versionID int64) ([]*TaskArtifact, error) {
	query := `
        SELECT id, task_version_id, type, artifact_key,
               COALESCE(file_path, ''), COALESCE(external_url, ''),
               COALESCE(original_filename, ''),
               mime_type, file_size, checksum, storage_type, status,
               error_message, created_at, updated_at
        FROM task_artifacts
        WHERE task_version_id = $1
        ORDER BY type, artifact_key
    `
	rows, err := db.Pool.Query(ctx, query, versionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get artifacts: %w", err)
	}
	defer rows.Close()

	var artifacts []*TaskArtifact
	for rows.Next() {
		var a TaskArtifact
		err := rows.Scan(
			&a.ID, &a.TaskVersionID, &a.Type, &a.ArtifactKey,
			&a.FilePath, &a.ExternalURL, &a.OriginalFilename,
			&a.MimeType, &a.FileSize, &a.Checksum, &a.StorageType, &a.Status,
			&a.ErrorMessage, &a.CreatedAt, &a.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan artifact: %w", err)
		}
		artifacts = append(artifacts, &a)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during artifacts iteration: %w", err)
	}

	return artifacts, nil
}