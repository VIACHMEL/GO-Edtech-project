package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

type ConditionArtifact struct {
	ID                          int64
	Kind, Key, Name, Path, MIME string
}

type TaskCondition struct {
	HTML      string
	Artifacts []ConditionArtifact
}

func (db *DB) TaskConditions(ctx context.Context, versions []int64) (map[int64]TaskCondition, error) {
	result := make(map[int64]TaskCondition, len(versions))
	if len(versions) == 0 {
		return result, nil
	}
	rows, err := db.Pool.Query(ctx, "SELECT id,condition_html FROM task_versions WHERE id=ANY($1)", versions)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var condition TaskCondition
		if err := rows.Scan(&id, &condition.HTML); err != nil {
			return nil, err
		}
		result[id] = condition
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	files, err := db.Pool.Query(ctx, `SELECT task_version_id,id,type,artifact_key,COALESCE(original_filename,''),
file_path,COALESCE(mime_type,'') FROM task_artifacts
WHERE task_version_id=ANY($1) AND status='done' AND storage_type='local' AND file_path IS NOT NULL
  AND (type='attachment' OR (type IN ('image','svg') AND artifact_key LIKE 'image\_%'))
ORDER BY id`, versions)
	if err != nil {
		return nil, err
	}
	defer files.Close()
	for files.Next() {
		var id int64
		var artifact ConditionArtifact
		if err := files.Scan(&id, &artifact.ID, &artifact.Kind, &artifact.Key, &artifact.Name, &artifact.Path, &artifact.MIME); err != nil {
			return nil, err
		}
		condition := result[id]
		condition.Artifacts = append(condition.Artifacts, artifact)
		result[id] = condition
	}
	if err := files.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (db *DB) AttemptItemVersion(ctx context.Context, actor, attemptID, itemID int64) (int64, error) {
	var version int64
	err := db.learningTx(ctx, actor, "", func(tx pgx.Tx) error {
		attempt, _, err := attemptForActor(ctx, tx, actor, attemptID)
		if err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT i.task_version_id
FROM attempt_answers a JOIN worksheet_items i ON i.id=a.worksheet_item_id
WHERE a.attempt_id=$1 AND i.id=$2 AND i.worksheet_id=$3`, attemptID, itemID, attempt.WorksheetID).Scan(&version)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return version, err
}
