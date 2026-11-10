
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    email TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    name TEXT NOT NULL,
    role TEXT NOT NULL,
    grade SMALLINT,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CHECK (role IN ('teacher', 'student', 'admin')),
    CHECK (status IN ('active', 'disabled')),
    CHECK (grade IS NULL OR grade BETWEEN 1 AND 12),
    CHECK (length(trim(email)) > 0),
    CHECK (length(trim(name)) > 0)
);

CREATE UNIQUE INDEX idx_users_email_lower
ON users (lower(email));

CREATE TABLE teacher_students (
    teacher_id BIGINT NOT NULL REFERENCES users(id),
    student_id BIGINT NOT NULL REFERENCES users(id),
    status TEXT NOT NULL DEFAULT 'active',
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (teacher_id, student_id),
    CHECK (teacher_id <> student_id),
    CHECK (status IN ('active', 'removed'))
);

CREATE INDEX idx_teacher_students_student
ON teacher_students(student_id);
