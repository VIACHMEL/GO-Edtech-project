DELETE FROM task_versions a
USING task_versions b
WHERE a.task_id = b.task_id
  AND a.version_number = b.version_number
  AND a.id < b.id;

ALTER TABLE task_versions
    ADD CONSTRAINT task_versions_task_version_unique
    UNIQUE (task_id, version_number);

ALTER TABLE task_artifacts
    ADD COLUMN external_url TEXT,
    ADD COLUMN original_filename TEXT,
    ADD COLUMN mime_type TEXT NOT NULL DEFAULT '',
    ADD COLUMN file_size BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN checksum TEXT NOT NULL DEFAULT '',
    ADD COLUMN storage_type TEXT NOT NULL DEFAULT 'local',
    ADD COLUMN error_message TEXT NOT NULL DEFAULT '';

ALTER TABLE task_artifacts ALTER COLUMN file_path DROP NOT NULL;

ALTER TABLE task_artifacts
    ADD CONSTRAINT artifact_done_has_source CHECK (
        status <> 'done'
        OR file_path IS NOT NULL
        OR (storage_type = 'external' AND external_url IS NOT NULL)
    );

ALTER TABLE task_artifacts
    ADD CONSTRAINT artifact_status_valid CHECK (
        status IN ('pending', 'done', 'failed', 'abandoned', 'external')
    );

ALTER TABLE task_artifacts
    ADD CONSTRAINT artifact_storage_valid CHECK (
        storage_type IN ('local', 's3', 'external')
    );

CREATE INDEX idx_task_artifacts_external_url
    ON task_artifacts(external_url)
    WHERE external_url IS NOT NULL;