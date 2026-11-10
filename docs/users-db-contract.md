# Блок 1 — БД пользователей, групп и приглашений

**Назначение:** слой хранения данных (`backend/pkg/db`) для основного Go-сервера. Миграции `000004`–`000006`. Локально пройдены 8 тестов (4 интеграционных) и `go vet ./pkg/db` — 10.11.2026. HTTP-интеграция и слияние с `main` ещё не подтверждены.

## 1. Таблицы PostgreSQL

| Таблица | Назначение и ключевые ограничения |
| --- | --- |
| `users` | Все аккаунты: `id`, `email`, `password_hash`, `name`, `role`, `grade?`, `status`, даты. Email уникален без учёта регистра. Роли `teacher/student` (`admin` предусмотрен SQL, но не `CreateUser`); статус `active/disabled`. |
| `teacher_students` | Связь преподавателя с учеником: `teacher_id`, `student_id`, `status`, `joined_at`. Составной PK `(teacher_id, student_id)`; статусы `active/removed`. |
| `groups` | Группы преподавателя: `id`, `teacher_id`, `name`, `subject?`, `status`, `created_at`. Статус `active/archived`. |
| `group_members` | Участники групп: `group_id`, `student_id`, `joined_at`. Составной PK `(group_id, student_id)`. |
| `invitations` | Одноразовые приглашения: `id`, `teacher_id`, `group_id?`, `invitee_email?`, `token_hash`, `status`, `expires_at`, `accepted_by?`, `accepted_at?`, `created_at`. Статусы `pending/accepted/revoked`; истечение вычисляется по времени. |

`?` — nullable (`NULL` в SQL). Все ссылки на пользователей/группы защищены внешними ключами. Роли и правила доступа дополнительно проверяются в запросах хранилища и в основном сервере.

## 2. Go API: методы `*db.DB`

Все методы принимают `context.Context`. `User` — профиль **без хеша пароля**, `UserCredentials` — внутренние учётные данные **с хешем**. `NewUser` содержит `Email`, `PasswordHash`, `Name`, `Role`, `Grade`.

**Пользователи (`users.go`):**

```go
CreateUser(ctx context.Context, input NewUser) (*User, error)
GetUser(ctx context.Context, id int64) (*User, error)
GetUserByEmail(ctx context.Context, email string) (*UserCredentials, error)
UpdateUserProfile(ctx context.Context, id int64, name string, grade *int16) (*User, error)
DisableUser(ctx context.Context, id int64) error
```

`CreateUser` записывает **готовый** хеш, нормализует email и проверяет роль. `DisableUser` помечает аккаунт `disabled`, но не удаляет историю.

**Преподаватель–ученик (`teacher_students.go`):**

```go
LinkTeacherStudent(ctx context.Context, teacherID, studentID int64) error
UnlinkTeacherStudent(ctx context.Context, teacherID, studentID int64) error
ListTeacherStudents(ctx context.Context, teacherID int64) ([]User, error)
ListStudentTeachers(ctx context.Context, studentID int64) ([]User, error)
```

`LinkTeacherStudent` создаёт/возобновляет связь активных пользователей правильных ролей. `UnlinkTeacherStudent` атомарно помечает связь `removed` и удаляет ученика из групп этого преподавателя. Повторное связывание **не** восстанавливает группы.

**Группы (`groups.go`, `group_models.go`):**

```go
CreateGroup(ctx context.Context, input NewGroup) (*Group, error)
GetGroup(ctx context.Context, groupID int64) (*Group, error)
UpdateGroup(ctx context.Context, groupID, teacherID int64, name string, subject *string) (*Group, error)
ListTeacherGroups(ctx context.Context, teacherID int64) ([]Group, error)
ArchiveGroup(ctx context.Context, groupID, teacherID int64) error
AddGroupMember(ctx context.Context, groupID, studentID int64) error
RemoveGroupMember(ctx context.Context, groupID, studentID int64) error
ListGroupMembers(ctx context.Context, groupID int64) ([]User, error)
```

`NewGroup` содержит `TeacherID`, `Name`, `Subject`. Добавление участника требует активной группы, преподавателя, ученика и связи `teacher_students`. `ArchiveGroup` скрывает группу из активных списков, не удаляя её строку.

**Приглашения (`invitations.go`):**

```go
CreateInvitation(ctx context.Context, input NewInvitation) (*Invitation, error)
GetInvitationByTokenHash(ctx context.Context, tokenHash string) (*Invitation, error)
RevokeInvitation(ctx context.Context, invitationID, teacherID int64) error
AcceptInvitation(ctx context.Context, tokenHash string, studentID int64) error
```

`NewInvitation` содержит `TeacherID`, `GroupID?`, `InviteeEmail?`, `TokenHash`, `ExpiresAt`. Сервер генерирует **криптографически случайный сырой токен**, вычисляет `SHA-256` и передаёт хранилищу его hex-хеш (64 символа). Сырой токен в БД не сохраняется.

`AcceptInvitation` рассчитан на **уже существующего авторизованного ученика**. В одной транзакции с `SELECT ... FOR UPDATE` метод проверяет срок, статус, роль, привязку к email/группе; создаёт связь, добавляет в группу (если указана), помечает приглашение `accepted`. Параллельные попытки принять тот же токен не создают повторной выдачи.

## 3. Использование сервером

```go
// При старте приложения (пример):
store, err := db.New(ctx, os.Getenv("DATABASE_URL"))
if err != nil { log.Fatal(err) }
defer store.Close()

// В обработчике после проверки сессии и прав:
students, err := store.ListTeacherStudents(requestCtx, authenticatedTeacherID)
// err необходимо обработать; затем преобразовать students в JSON-ответ.
```

`db.New`/`Close` существовали до блока 1; соединение через `pgx/v5` и `pgxpool`, ID — `int64`/SQL `BIGINT`.

**Граница ответственности:** данный модуль не реализует HTTP, сессии, регистрацию как веб-сценарий, смену/проверку паролей, генерацию токенов и рассылку приглашений. Это ответственность сервера/авторизации. Нельзя передавать пользовательские `teacherID`/`studentID` из JSON без проверки; проверять владение объектом перед `GetGroup`, `GetUser`, `ListGroupMembers`, `RemoveGroupMember` и прочими операциями. `GetUserByEmail` и `GetInvitationByTokenHash` — внутренние операции. Никогда не возвращать `PasswordHash` и токены на frontend.

**Ошибки:** использовать `errors.Is(err, db.ErrNotFound)`, `errors.Is(err, db.ErrValidation)`, `errors.Is(err, db.ErrConflict)`. Обычно они соответствуют HTTP `404`, `400/422`, `409`, но сервер сам решает, какие детали раскрывать. `401/403` определяются сервером при проверке пользователя/прав. `UpdateUserProfile` может возвращать `ErrNotFound`, если попытаться указать класс преподавателю.

## 4. Тестирование, передача, ограничения

Отдельная БД `edtech_test_db` с миграциями `000004`–`000006`:

```bash
cd backend
export EDTECH_TEST_DSN='postgres://edtech_user:YOUR_LOCAL_PASSWORD@127.0.0.1:15432/edtech_test_db?sslmode=disable'
go test ./pkg/db -run Integration -v -count=1
go test ./pkg/db -v -count=1
go vet ./pkg/db
```

Проверено: создание/поиск пользователей, email, роли, группы, привязка/отвязка, приглашения, отзыв, срок действия, повторные и одновременные запросы. **Не проверено в этом блоке:** HTTP-авторизация, сквозные права доступа, CI и PR/слияние в `main`. Ранее `go test ./...` выявлял отдельную проблему в `cmd/kompege` — не относится к данному пакету.

**До подключения сервера согласовать:** кто и как создаёт аккаунты учеников, как реализовать изменение/сброс пароля (метода `UpdatePasswordHash` пока нет), как отзывать сессии отключённых аккаунтов и какие операции доступны преподавателю/ученику при архивировании группы.
