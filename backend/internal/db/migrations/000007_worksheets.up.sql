CREATE TABLE worksheets (
    id BIGSERIAL PRIMARY KEY,
    teacher_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    title TEXT NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 200),
    subject TEXT NOT NULL CHECK (length(btrim(subject)) BETWEEN 1 AND 64),
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','published','archived')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (status <> 'published' OR published_at IS NOT NULL),
    CHECK (status <> 'draft' OR published_at IS NULL)
);
CREATE INDEX idx_worksheets_teacher_id ON worksheets(teacher_id, id DESC);

CREATE TABLE worksheet_items (
    id BIGSERIAL PRIMARY KEY,
    worksheet_id BIGINT NOT NULL REFERENCES worksheets(id) ON DELETE CASCADE,
    task_version_id BIGINT NOT NULL REFERENCES task_versions(id) ON DELETE RESTRICT,
    position INT NOT NULL CHECK (position > 0),
    max_score NUMERIC(6,2) NOT NULL CHECK (max_score >= 0),
    settings JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(settings) = 'object'),
    content_snapshot TEXT,
    answer_snapshot TEXT,
    solution_snapshot TEXT,
    task_type_snapshot TEXT,
    CONSTRAINT worksheet_items_position_key UNIQUE(worksheet_id, position) DEFERRABLE INITIALLY IMMEDIATE
);
CREATE INDEX idx_worksheet_items_version ON worksheet_items(task_version_id);
