DROP TABLE board_operations, board_links, boards;
ALTER TABLE worksheet_items DROP COLUMN exam_part_snapshot;
DROP FUNCTION task_exam_part(TEXT, TEXT, INT, JSONB);
