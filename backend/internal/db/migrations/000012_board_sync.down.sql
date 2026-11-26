-- The old editor cannot display image elements. Keep the rest of each scene.
UPDATE boards b
SET snapshot = COALESCE((
    SELECT jsonb_agg(e ORDER BY position)
    FROM jsonb_array_elements(b.snapshot) WITH ORDINALITY AS s(e, position)
    WHERE e->>'type' <> 'image'
), '[]'::jsonb)
WHERE EXISTS (
    SELECT 1 FROM jsonb_array_elements(b.snapshot) e WHERE e->>'type' = 'image'
);

DROP TABLE board_files;

-- The full operation payloads cannot be recovered from the retained hashes.
DELETE FROM board_operations;
ALTER TABLE board_operations
    ADD COLUMN payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    DROP COLUMN payload_digest,
    DROP COLUMN base_seq;

ALTER TABLE boards
    DROP COLUMN history_floor,
    DROP COLUMN element_versions;
