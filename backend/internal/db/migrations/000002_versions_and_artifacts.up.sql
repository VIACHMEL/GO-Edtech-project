CREATE TABLE task_versions (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    content_text TEXT NOT NULL,
    answer TEXT NOT NULL,
    solution TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    version_number INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(task_id, content_hash)
);

CREATE TABLE task_artifacts (
    id BIGSERIAL PRIMARY KEY,
    task_version_id BIGINT NOT NULL REFERENCES task_versions(id) ON DELETE CASCADE,
    type TEXT NOT NULL,                                   -- "svg", "tikz", "preview"
    artifact_key TEXT NOT NULL DEFAULT 'main',            -- "main", "figure_1", "figure_2", "solution_graph"
    file_path TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT task_artifacts_version_key_unique
        UNIQUE (task_version_id, type, artifact_key)
);

ALTER TABLE tasks ADD COLUMN current_version_id BIGINT REFERENCES task_versions(id);

INSERT INTO task_versions (task_id, content_text, answer, solution, content_hash, version_number)
SELECT id, content_text, answer, solution, content_hash, 1
FROM tasks;

UPDATE tasks t
SET current_version_id = tv.id
FROM task_versions tv
WHERE t.id = tv.task_id;

ALTER TABLE tasks
    DROP COLUMN content_text,
    DROP COLUMN answer,
    DROP COLUMN solution,
    DROP COLUMN content_hash;

ALTER TABLE tasks ADD CONSTRAINT unique_source_url UNIQUE (source_url);

CREATE INDEX idx_task_versions_task_id ON task_versions(task_id);
CREATE INDEX idx_task_artifacts_version_id ON task_artifacts(task_version_id);
CREATE INDEX idx_task_artifacts_status ON task_artifacts(status);

CREATE OR REPLACE FUNCTION update_current_version()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE tasks
    SET current_version_id = NEW.id
    WHERE id = NEW.task_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_update_current_version
AFTER INSERT OR UPDATE ON task_versions
FOR EACH ROW
EXECUTE FUNCTION update_current_version();