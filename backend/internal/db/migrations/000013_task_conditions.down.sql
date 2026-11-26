-- Снимки досок и рисунки уже опубликованных вариантов сохраняются.
CREATE OR REPLACE FUNCTION task_exam_part(exam TEXT, subject_code TEXT, task_no INT, meta JSONB)
RETURNS SMALLINT LANGUAGE SQL IMMUTABLE AS $$
    SELECT CASE
        WHEN meta->'exam_part' = '1'::jsonb THEN 1
        WHEN meta->'exam_part' = '2'::jsonb THEN 2
        WHEN exam = 'oge' AND subject_code = 'math' AND task_no BETWEEN 1 AND 25
            THEN CASE WHEN task_no < 20 THEN 1 ELSE 2 END
    END::SMALLINT
$$;

ALTER TABLE task_versions DROP COLUMN condition_html;
