CREATE FUNCTION task_exam_part(exam TEXT, subject_code TEXT, task_no INT, meta JSONB)
RETURNS SMALLINT LANGUAGE SQL IMMUTABLE AS $$
    SELECT CASE
        WHEN meta->'exam_part' = '1'::jsonb THEN 1
        WHEN meta->'exam_part' = '2'::jsonb THEN 2
        WHEN exam = 'oge' AND subject_code = 'math' AND task_no BETWEEN 1 AND 25
            THEN CASE WHEN task_no < 20 THEN 1 ELSE 2 END
    END::SMALLINT
$$;

ALTER TABLE worksheet_items ADD COLUMN exam_part_snapshot SMALLINT
    CHECK (exam_part_snapshot IN (1, 2));

UPDATE worksheet_items i
SET exam_part_snapshot = task_exam_part(t.exam_type, t.subject, t.task_number, t.metadata)
FROM task_versions v JOIN tasks t ON t.id = v.task_id, worksheets w
WHERE v.id = i.task_version_id AND w.id = i.worksheet_id AND w.published_at IS NOT NULL;

CREATE TABLE boards (
    id BIGSERIAL PRIMARY KEY,
    owner_id BIGINT NOT NULL REFERENCES users(id),
    snapshot JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(snapshot) = 'array'),
    snapshot_seq BIGINT NOT NULL DEFAULT 0 CHECK (snapshot_seq >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- A NULL position is the test-part board shared within one attempt.
CREATE TABLE board_links (
    board_id BIGINT PRIMARY KEY REFERENCES boards(id) ON DELETE CASCADE,
    attempt_id BIGINT NOT NULL,
    worksheet_id BIGINT NOT NULL,
    worksheet_item_id BIGINT,
    UNIQUE NULLS NOT DISTINCT (attempt_id, worksheet_item_id),
    FOREIGN KEY (attempt_id, worksheet_id) REFERENCES attempts(id, worksheet_id),
    FOREIGN KEY (worksheet_item_id, worksheet_id) REFERENCES worksheet_items(id, worksheet_id)
);

CREATE TABLE board_operations (
    id BIGSERIAL PRIMARY KEY,
    board_id BIGINT NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    seq BIGINT NOT NULL CHECK (seq > 0),
    op_id UUID NOT NULL UNIQUE,
    author_id BIGINT NOT NULL REFERENCES users(id),
    payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (board_id, seq)
);
