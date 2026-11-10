# GO-Edtech — таблицы PostgreSQL и поля

**Условные обозначения:** `PK` — первичный ключ; `FK →` — внешний ключ; `UQ` — уникально; `?` — допускает `NULL`. `BIGINT` ↔ `int64` в Go; время — `TIMESTAMPTZ` (UTC в API); `JSONB` — структурированные данные. **Это проектная структура, не готовая SQL-миграция.** Таблицы 1–3 банка уже разрабатываются в `features/task-database`; остальные новые. Этапы: **A** базовый MVP; **B** доска/занятия; **C** позже.

## Пользователи — A

**`users`**
- `id BIGINT PK`, `email TEXT UQ`, `password_hash TEXT`, `name TEXT`, `role TEXT` (`teacher/student/admin`), `grade SMALLINT?`, `status TEXT`, `created_at TIMESTAMPTZ`, `updated_at TIMESTAMPTZ`.

**`teacher_students`**
- `teacher_id BIGINT FK → users.id`, `student_id BIGINT FK → users.id`, `status TEXT`, `joined_at TIMESTAMPTZ`; **PK (`teacher_id`, `student_id`)**.

**`groups`**
- `id BIGINT PK`, `teacher_id BIGINT FK → users.id`, `name TEXT`, `subject TEXT?`, `status TEXT`, `created_at TIMESTAMPTZ`.

**`group_members`**
- `group_id BIGINT FK → groups.id`, `student_id BIGINT FK → users.id`, `joined_at TIMESTAMPTZ`; **PK (`group_id`, `student_id`)**.

**`invitations`**
- `id BIGINT PK`, `teacher_id BIGINT FK → users.id`, `group_id BIGINT? FK → groups.id`, `invitee_email TEXT?`, `token_hash TEXT UQ`, `status TEXT`, `expires_at TIMESTAMPTZ`, `accepted_by BIGINT? FK → users.id`, `accepted_at TIMESTAMPTZ?`.

## Банк задач — A (уже есть основа)

**`tasks`** ✓
- `id BIGINT PK`, `source_url TEXT UQ`, `source_site TEXT`, `subject TEXT`, `task_number INT`, `year INT`, `exam_type TEXT`, `task_type TEXT`, `metadata JSONB`, `current_version_id BIGINT? FK → task_versions.id`, `created_at TIMESTAMPTZ`, `updated_at TIMESTAMPTZ`.

**`task_versions`** ✓
- `id BIGINT PK`, `task_id BIGINT FK → tasks.id`, `version_number INT`, `content_text TEXT`, `answer TEXT`, `solution TEXT`, `content_hash TEXT`, `created_at TIMESTAMPTZ`; **UQ (`task_id`, `version_number`), UQ (`task_id`, `content_hash`)**.

**`task_artifacts`** ✓
- `id BIGINT PK`, `task_version_id BIGINT FK → task_versions.id`, `type TEXT` (`image/svg/tikz/preview/attachment`), `artifact_key TEXT`, `storage_type TEXT`, `file_path TEXT?`, `external_url TEXT?`, `original_filename TEXT?`, `mime_type TEXT`, `file_size BIGINT`, `checksum TEXT`, `status TEXT`, `error_message TEXT`, `created_at TIMESTAMPTZ`, `updated_at TIMESTAMPTZ`; **UQ (`task_version_id`, `type`, `artifact_key`)**.
- *При доработке для SVG → TikZ:* `review_status TEXT` (`needs_review/approved/rejected`), `converter_version TEXT?`, `source_artifact_id BIGINT? FK → task_artifacts.id` — **планируемые** поля, в текущих миграциях их нет.

## Варианты и выполнение — A

**`worksheets`**
- `id BIGINT PK`, `teacher_id BIGINT FK → users.id`, `title TEXT`, `subject TEXT`, `status TEXT` (`draft/published/archived`), `created_at TIMESTAMPTZ`, `published_at TIMESTAMPTZ?`, `updated_at TIMESTAMPTZ`.

**`worksheet_items`**
- `id BIGINT PK`, `worksheet_id BIGINT FK → worksheets.id`, `task_version_id BIGINT FK → task_versions.id`, `position INT`, `max_score NUMERIC(6,2)`, `settings JSONB`; **UQ (`worksheet_id`, `position`)**.

**`assignments`**
- `id BIGINT PK`, `worksheet_id BIGINT FK → worksheets.id`, `student_id BIGINT FK → users.id`, `assigned_by BIGINT FK → users.id`, `group_id BIGINT? FK → groups.id`, `distribution_id UUID` (общий для одной массовой выдачи), `due_at TIMESTAMPTZ?`, `status TEXT` (`active/closed/cancelled`), `assigned_at TIMESTAMPTZ`.

**`attempts`**
- `id BIGINT PK`, `assignment_id BIGINT FK → assignments.id`, `attempt_no INT`, `status TEXT` (`draft/submitted/reviewed`), `started_at TIMESTAMPTZ`, `submitted_at TIMESTAMPTZ?`, `result_published_at TIMESTAMPTZ?`; **UQ (`assignment_id`, `attempt_no`)**.

**`attempt_answers`**
- `id BIGINT PK`, `attempt_id BIGINT FK → attempts.id`, `worksheet_item_id BIGINT FK → worksheet_items.id`, `part_key TEXT` (по умолчанию `''`; номер подзадачи), `answer_text TEXT?`, `answer_data JSONB?`, `is_correct BOOLEAN?`, `score NUMERIC(6,2)?`, `grade_status TEXT`, `answer_revision INT`, `graded_revision INT?`, `graded_by BIGINT? FK → users.id`, `teacher_comment TEXT?`, `updated_at TIMESTAMPTZ`; **UQ (`attempt_id`, `worksheet_item_id`, `part_key`)**.

## Файлы и доска — A/B

**`files`** — пользовательские вложения, НЕ `task_artifacts`
- `id BIGINT PK`, `uploaded_by BIGINT FK → users.id`, `storage_key TEXT UQ`, `original_name TEXT`, `mime_type TEXT`, `size_bytes BIGINT`, `checksum TEXT`, `answer_id BIGINT? FK → attempt_answers.id`, `worksheet_id BIGINT? FK → worksheets.id`, `board_id BIGINT? FK → boards.id`, `lesson_id BIGINT? FK → lessons.id`, `created_at TIMESTAMPTZ`.
- **CHECK:** ровно один из `answer_id/worksheet_id/board_id/lesson_id` заполнен; данные файла — во внешнем хранилище. Для ссылок на оригиналы задач используется `task_artifacts`.

**`boards`**
- `id BIGINT PK`, `owner_id BIGINT FK → users.id`, `title TEXT`, `snapshot JSONB` (объекты/слои/позиции/ссылки на файлы), `snapshot_seq BIGINT`, `revision BIGINT`, `created_at TIMESTAMPTZ`, `updated_at TIMESTAMPTZ`.

**`board_links`** — привязка одной доски к разным объектам
- `id BIGINT PK`, `board_id BIGINT FK → boards.id`, `target_type TEXT` (`task_version/worksheet/worksheet_item/assignment/assignment_item/lesson`), `task_version_id BIGINT? FK → task_versions.id`, `worksheet_id BIGINT? FK → worksheets.id`, `worksheet_item_id BIGINT? FK → worksheet_items.id`, `assignment_id BIGINT? FK → assignments.id`, `lesson_id BIGINT? FK → lessons.id`, `access_mode TEXT` (`teacher_only/student_read/student_write/collaborative`).
- **CHECK:** одна цель, либо **ровно пара** `assignment_id + worksheet_item_id` для личной доски конкретного задания. Сервер дополнительно проверяет, что `worksheet_item_id` принадлежит варианту назначения.

**`board_operations`**
- `id BIGINT PK`, `board_id BIGINT FK → boards.id`, `seq BIGINT`, `op_id UUID UQ` (повтор отправки операции), `author_id BIGINT FK → users.id`, `payload JSONB`, `created_at TIMESTAMPTZ`; **UQ (`board_id`, `seq`)**.

## Занятия — B

**`lessons`**
- `id BIGINT PK`, `teacher_id BIGINT FK → users.id`, `group_id BIGINT? FK → groups.id`, `title TEXT`, `starts_at TIMESTAMPTZ`, `ends_at TIMESTAMPTZ`, `status TEXT` (`scheduled/active/finished/cancelled`), `video_provider TEXT?`, `video_room_id TEXT?`, `created_at TIMESTAMPTZ`.

**`lesson_participants`**
- `lesson_id BIGINT FK → lessons.id`, `user_id BIGINT FK → users.id`, `attendance TEXT` (`invited/present/absent`), `joined_at TIMESTAMPTZ?`; **PK (`lesson_id`, `user_id`)**.

## Позже — C (пока не создавать миграции)

**`notifications`**
- `id BIGINT PK`, `user_id BIGINT FK → users.id`, `kind TEXT`, `payload JSONB`, `created_at TIMESTAMPTZ`, `read_at TIMESTAMPTZ?`.

**`subscriptions`**
- `id BIGINT PK`, `teacher_id BIGINT FK → users.id`, `plan_code TEXT`, `provider_subscription_id TEXT?`, `status TEXT`, `period_start TIMESTAMPTZ`, `period_end TIMESTAMPTZ`, `cancelled_at TIMESTAMPTZ?`.

**`payments`**
- `id BIGINT PK`, `teacher_id BIGINT FK → users.id`, `subscription_id BIGINT? FK → subscriptions.id`, `provider_payment_id TEXT UQ`, `amount_minor BIGINT`, `currency CHAR(3)`, `status TEXT`, `created_at TIMESTAMPTZ`, `paid_at TIMESTAMPTZ?`.

**`ai_jobs`**
- `id BIGINT PK`, `job_type TEXT`, `requested_by BIGINT? FK → users.id`, `answer_id BIGINT? FK → attempt_answers.id`, `task_artifact_id BIGINT? FK → task_artifacts.id`, `model_version TEXT?`, `status TEXT`, `input_ref JSONB`, `result JSONB?`, `error TEXT?`, `created_at TIMESTAMPTZ`, `finished_at TIMESTAMPTZ?`.

## Минимальные ограничения и индексы

- Все `FK` реально создаются в PostgreSQL; индексы на связи, по которым часто идёт поиск: `teacher_students.student_id`, `worksheet_items.worksheet_id`, `assignments.student_id`, `attempts.assignment_id`, `attempt_answers.attempt_id`, `board_links.board_id`, `board_operations(board_id,seq)`, `lessons(teacher_id,starts_at)`.
- Для `board_links`: индекс на каждый используемый тип цели; отсутствие дублей связей — через частичные уникальные индексы/валидацию; проверка доступа по цели обязательна.
- `task_versions`, `worksheets` после публикации, `attempts` после сдачи — не редактируются произвольно. Новая версия/попытка вместо переписывания истории.
- Проверка `attempt_answers.worksheet_item_id` принадлежит варианту из `attempts.assignment_id`; проверка `score` в допустимых пределах; сохранение и проверка по ревизиям.
- `−1 / 0 / 1` **не отдельная колонка**: `−1` нет ответа, `0` неверно, `1` верно; `NULL` — есть ответ, но ещё не проверен. Для студента эти значения до публикации скрыты на сервере.
- Файлы и доски не должны содержать секреты, токены видеосервиса и большие base64-данные. Для платёжных данных не хранить номера банковских карт; обрабатывать webhook идемпотентно.
