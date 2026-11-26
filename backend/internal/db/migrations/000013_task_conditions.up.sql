ALTER TABLE task_versions ADD COLUMN condition_html TEXT NOT NULL DEFAULT '';

UPDATE task_versions v SET condition_html = t.metadata->>'content_html'
FROM tasks t
WHERE t.current_version_id = v.id AND t.id = v.task_id AND t.source_site = 'kompege.ru'
  AND jsonb_typeof(t.metadata->'content_html') = 'string';

CREATE OR REPLACE FUNCTION task_exam_part(exam TEXT, subject_code TEXT, task_no INT, meta JSONB)
RETURNS SMALLINT LANGUAGE SQL IMMUTABLE AS $$
    SELECT CASE
        WHEN meta->'exam_part' = '1'::jsonb THEN 1
        WHEN meta->'exam_part' = '2'::jsonb THEN 2
        WHEN exam = 'oge' AND subject_code = 'math' AND task_no BETWEEN 1 AND 25
            THEN CASE WHEN task_no < 20 THEN 1 ELSE 2 END
        WHEN exam = 'ege' AND subject_code = 'informatics' AND task_no BETWEEN 1 AND 27
            THEN CASE WHEN task_no <= 23 THEN 1 ELSE 2 END
    END::SMALLINT
$$;

UPDATE worksheet_items i
SET exam_part_snapshot = task_exam_part(t.exam_type,t.subject,t.task_number,t.metadata)
FROM task_versions v JOIN tasks t ON t.id=v.task_id, worksheets w
WHERE i.task_version_id=v.id AND i.worksheet_id=w.id AND w.published_at IS NOT NULL
  AND i.exam_part_snapshot IS NULL
  AND t.exam_type='ege' AND t.subject='informatics';
