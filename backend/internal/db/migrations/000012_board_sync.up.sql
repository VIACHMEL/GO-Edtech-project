ALTER TABLE boards
    ADD COLUMN element_versions JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(element_versions) = 'object'),
    ADD COLUMN history_floor BIGINT NOT NULL DEFAULT 0
        CHECK (history_floor BETWEEN 0 AND snapshot_seq);

-- Versions before this migration were tracked per board. Existing clients
-- can still replay retained receipts, but must get a fresh base before editing.
UPDATE boards AS b
SET history_floor = b.snapshot_seq,
    element_versions = COALESCE((
        SELECT jsonb_object_agg(e->>'id', b.snapshot_seq)
        FROM jsonb_array_elements(b.snapshot) e
        WHERE NOT (e @> '{"isDeleted":true}'::jsonb)
    ), '{}'::jsonb),
    snapshot = COALESCE((
        SELECT jsonb_agg(e ORDER BY position)
        FROM jsonb_array_elements(b.snapshot) WITH ORDINALITY AS s(e, position)
        WHERE NOT (e @> '{"isDeleted":true}'::jsonb)
    ), '[]'::jsonb)
WHERE b.snapshot_seq > 0;

DELETE FROM board_operations o USING boards b
WHERE o.board_id = b.id AND o.seq <= b.snapshot_seq - 512;

ALTER TABLE board_operations
    ADD COLUMN base_seq BIGINT NOT NULL DEFAULT 0 CHECK (base_seq >= 0 AND base_seq < seq),
    ADD COLUMN payload_digest BYTEA CHECK (octet_length(payload_digest) = 32);

UPDATE board_operations SET payload_digest = sha256(convert_to(payload::text, 'UTF8'));

ALTER TABLE board_operations
    ALTER COLUMN payload_digest SET NOT NULL,
    DROP COLUMN payload;

CREATE TABLE board_files (
    board_id BIGINT NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    file_id TEXT NOT NULL CHECK (file_id ~ '^[A-Za-z0-9_-]{1,128}$'),
    mime TEXT NOT NULL CHECK (mime IN ('image/png', 'image/jpeg')),
    content BYTEA NOT NULL CHECK (octet_length(content) BETWEEN 1 AND 2097152),
    PRIMARY KEY (board_id, file_id)
);
