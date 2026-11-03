CREATE TABLE tasks (
    id BIGSERIAL PRIMARY KEY,
    source_url TEXT NOT NULL,
    source_site TEXT NOT NULL,
    subject TEXT NOT NULL,
    task_number INT NOT NULL,
    content_text TEXT NOT NULL,
    answer TEXT NOT NULL,
    year INT NOT NULL DEFAULT 0,
    exam_type TEXT NOT NULL DEFAULT '',
    task_type TEXT NOT NULL DEFAULT '',
    solution TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    content_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    
    UNIQUE(source_url, content_hash)
);

CREATE INDEX idx_tasks_subject ON tasks(subject);
CREATE INDEX idx_tasks_year ON tasks(year);

CREATE INDEX idx_tasks_metadata ON tasks USING gin (metadata);