ALTER TABLE worksheets ADD COLUMN max_attempts INT NOT NULL DEFAULT 1 CHECK(max_attempts > 0);
ALTER TABLE assignments ADD COLUMN max_attempts INT NOT NULL DEFAULT 1 CHECK(max_attempts > 0);
UPDATE assignments a SET max_attempts=w.max_attempts FROM worksheets w WHERE w.id=a.worksheet_id;
ALTER TABLE attempts ADD COLUMN is_late BOOLEAN NOT NULL DEFAULT false;
-- Historical submission time is preserved; deadlines are the values currently stored.
UPDATE attempts t SET is_late=(a.due_at IS NOT NULL AND t.submitted_at>a.due_at)
FROM assignments a WHERE a.id=t.assignment_id AND t.submitted_at IS NOT NULL;
