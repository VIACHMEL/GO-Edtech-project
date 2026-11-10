DROP INDEX IF EXISTS idx_task_artifacts_external_url;

ALTER TABLE task_artifacts
    DROP CONSTRAINT IF EXISTS artifact_storage_valid,
    DROP CONSTRAINT IF EXISTS artifact_status_valid,
    DROP CONSTRAINT IF EXISTS artifact_done_has_source,
    DROP COLUMN IF EXISTS storage_type,
    DROP COLUMN IF EXISTS checksum,
    DROP COLUMN IF EXISTS file_size,
    DROP COLUMN IF EXISTS mime_type,
    DROP COLUMN IF EXISTS original_filename,
    DROP COLUMN IF EXISTS external_url,
    DROP COLUMN IF EXISTS error_message;

UPDATE task_artifacts SET file_path = '' WHERE file_path IS NULL;
ALTER TABLE task_artifacts ALTER COLUMN file_path SET NOT NULL;

ALTER TABLE task_versions DROP CONSTRAINT IF EXISTS task_versions_task_version_unique;