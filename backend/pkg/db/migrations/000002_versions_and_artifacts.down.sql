DROP TRIGGER IF EXISTS trigger_update_current_version ON task_versions;
DROP FUNCTION IF EXISTS update_current_version();

ALTER TABLE tasks 
    ADD COLUMN content_text TEXT NOT NULL DEFAULT '',
    ADD COLUMN answer TEXT NOT NULL DEFAULT '',
    ADD COLUMN solution TEXT NOT NULL DEFAULT '',
    ADD COLUMN content_hash TEXT NOT NULL DEFAULT '';

UPDATE tasks t
SET content_text = tv.content_text,
    answer = tv.answer,
    solution = tv.solution,
    content_hash = tv.content_hash
FROM task_versions tv
WHERE t.current_version_id = tv.id;

ALTER TABLE tasks DROP CONSTRAINT unique_source_url;
ALTER TABLE tasks ADD CONSTRAINT tasks_source_url_content_hash_key UNIQUE (source_url, content_hash);

ALTER TABLE tasks DROP COLUMN current_version_id;

DROP TABLE IF EXISTS task_artifacts;
DROP TABLE IF EXISTS task_versions;