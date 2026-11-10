-- Учебные группы одного преподавателя.
CREATE TABLE groups (
    id BIGSERIAL PRIMARY KEY,
    teacher_id BIGINT NOT NULL REFERENCES users(id),
    name TEXT NOT NULL CHECK (length(btrim(name)) > 0),
    subject TEXT,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_groups_teacher ON groups(teacher_id);

-- Каждый ученик может состоять во многих группах.
CREATE TABLE group_members (
    group_id BIGINT NOT NULL REFERENCES groups(id),
    student_id BIGINT NOT NULL REFERENCES users(id),
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (group_id, student_id)
);

CREATE INDEX idx_group_members_student ON group_members(student_id);
