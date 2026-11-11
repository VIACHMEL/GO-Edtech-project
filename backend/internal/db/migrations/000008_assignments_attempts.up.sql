ALTER TABLE worksheet_items ADD CONSTRAINT worksheet_items_id_worksheet_key UNIQUE(id,worksheet_id);
CREATE TABLE assignments (
 id BIGSERIAL PRIMARY KEY,
 worksheet_id BIGINT NOT NULL REFERENCES worksheets(id) ON DELETE RESTRICT,
 student_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 assigned_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 group_id BIGINT REFERENCES groups(id) ON DELETE RESTRICT,
 distribution_id UUID NOT NULL,
 due_at TIMESTAMPTZ,
 status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','closed','cancelled')),
 assigned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(distribution_id,student_id),
 UNIQUE(id,worksheet_id)
);
CREATE INDEX idx_assignments_student ON assignments(student_id,id DESC);
CREATE INDEX idx_assignments_teacher ON assignments(assigned_by,id DESC);
CREATE INDEX idx_assignments_worksheet ON assignments(worksheet_id);
CREATE INDEX idx_assignments_group ON assignments(group_id) WHERE group_id IS NOT NULL;
CREATE TABLE attempts (
 id BIGSERIAL PRIMARY KEY,
 assignment_id BIGINT NOT NULL,
 worksheet_id BIGINT NOT NULL,
 attempt_no INT NOT NULL CHECK(attempt_no>0),
 status TEXT NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','submitted','reviewed')),
 started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 submitted_at TIMESTAMPTZ,
 result_published_at TIMESTAMPTZ,
 FOREIGN KEY(assignment_id,worksheet_id) REFERENCES assignments(id,worksheet_id) ON DELETE RESTRICT,
 UNIQUE(assignment_id,attempt_no),
 UNIQUE(id,worksheet_id),
 CHECK((status='draft' AND submitted_at IS NULL AND result_published_at IS NULL)
    OR (status='submitted' AND submitted_at IS NOT NULL AND result_published_at IS NULL)
    OR (status='reviewed' AND submitted_at IS NOT NULL AND result_published_at IS NOT NULL))
);
CREATE UNIQUE INDEX idx_attempts_one_draft ON attempts(assignment_id) WHERE status='draft';
CREATE TABLE attempt_answers (
 id BIGSERIAL PRIMARY KEY,
 attempt_id BIGINT NOT NULL,
 worksheet_id BIGINT NOT NULL,
 worksheet_item_id BIGINT NOT NULL,
 part_key TEXT NOT NULL DEFAULT '' CHECK(part_key=''),
 answer_text TEXT,
 answer_data JSONB,
 is_correct BOOLEAN,
 score NUMERIC(6,2) CHECK(score>=0),
 grade_status TEXT NOT NULL DEFAULT 'pending' CHECK(grade_status IN ('pending','auto','manual')),
 answer_revision INT NOT NULL DEFAULT 0 CHECK(answer_revision>=0),
 graded_revision INT,
 graded_by BIGINT REFERENCES users(id) ON DELETE RESTRICT,
 teacher_comment TEXT,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(attempt_id,worksheet_id) REFERENCES attempts(id,worksheet_id) ON DELETE RESTRICT,
 FOREIGN KEY(worksheet_item_id,worksheet_id) REFERENCES worksheet_items(id,worksheet_id) ON DELETE RESTRICT,
 UNIQUE(attempt_id,worksheet_item_id,part_key),
 CHECK((grade_status='pending' AND graded_revision IS NULL AND score IS NULL AND is_correct IS NULL)
    OR (grade_status IN ('auto','manual') AND graded_revision=answer_revision AND score IS NOT NULL AND is_correct IS NOT NULL))
);
CREATE INDEX idx_attempt_answers_item ON attempt_answers(worksheet_item_id);
