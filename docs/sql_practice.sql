-- Execute only after migration 000004 in a LOCAL development database.
-- All example rows are rolled back at the end.
BEGIN;

INSERT INTO users (email, password_hash, name, role, grade)
VALUES ('demo.teacher@example.invalid', 'demo_hash_not_for_login', 'Test teacher', 'teacher', NULL),
       ('demo.student@example.invalid', 'demo_hash_not_for_login', 'Test student', 'student', 11)
RETURNING id, email, role;

INSERT INTO teacher_students (teacher_id, student_id)
SELECT t.id, s.id
FROM users AS t
CROSS JOIN users AS s
WHERE t.email = 'demo.teacher@example.invalid'
  AND s.email = 'demo.student@example.invalid';

SELECT teacher.name AS teacher, student.name AS student, ts.status
FROM teacher_students AS ts
JOIN users AS teacher ON teacher.id = ts.teacher_id
JOIN users AS student ON student.id = ts.student_id
WHERE teacher.email = 'demo.teacher@example.invalid';

UPDATE teacher_students SET status = 'removed'
WHERE teacher_id = (SELECT id FROM users WHERE email = 'demo.teacher@example.invalid')
  AND student_id = (SELECT id FROM users WHERE email = 'demo.student@example.invalid');

SELECT teacher_id, student_id, status FROM teacher_students
WHERE teacher_id = (SELECT id FROM users WHERE email = 'demo.teacher@example.invalid');

ROLLBACK;
