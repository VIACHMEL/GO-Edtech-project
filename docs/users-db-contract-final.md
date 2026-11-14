# GO-Edtech — контракт БД пользователей, связей, групп и приглашений

**Версия:** 2.0 · **Источник:** реализация `main` на 14.11.2026 (`fe8f983`)  
**Пакет Go:** `github.com/VIACHMEL/GO-Edtech-project/backend/internal/db`  
**Для кого:** разработчик основного Go-сервера и авторизации. Это **контракт слоя хранения**, а не HTTP/OpenAPI/JSON-контракт.

## Содержание

1. [Границы ответственности](#1-границы-ответственности)
2. [Пять таблиц PostgreSQL](#2-пять-таблиц-postgresql)
3. [Структуры Go и их поля](#3-структуры-go-и-их-поля)
4. [Все методы: параметры, поведение, ошибки](#4-все-методы-параметры-поведение-ошибки)
5. [Общие ошибки и ограничения безопасности](#5-общие-ошибки-и-ограничения-безопасности)
6. [Подключение, тесты и открытые задачи](#6-подключение-тесты-и-открытые-задачи)


## 2. Пять таблиц PostgreSQL

Миграции находятся в `backend/internal/db/migrations/` и применяются **по порядку**: `000004_users_and_teacher_students`, `000005_groups`, `000006_invitations`. Общая БД уже содержит `tasks`, `task_versions`, `task_artifacts` из `000001–000003`.

### `users` — общие аккаунты

| Поле | Тип | Назначение и ограничение |
|---|---|---|
| `id` | `BIGSERIAL PK` | Автоматический ID пользователя. |
| `email` | `TEXT NOT NULL` | Email для поиска/входа; уникальный индекс по `lower(email)` (без учёта регистра). В Go приводится к нижнему регистру и проверяется формат. |
| `password_hash` | `TEXT NOT NULL` | **Готовый** хеш пароля от auth-сервиса; исходный пароль не хранится. SQL не проверяет криптостойкость хеша. |
| `name` | `TEXT NOT NULL` | Отображаемое имя, без пустых/пробельных строк. |
| `role` | `TEXT NOT NULL` | SQL допускает `teacher`, `student`, `admin`; `CreateUser` сейчас создаёт **только** `teacher` и `student`. |
| `grade` | `SMALLINT NULL` | Класс `1..12` или `NULL`; Go разрешает заполнение только ученику. |
| `status` | `TEXT NOT NULL` | `active` (по умолчанию) или `disabled`. |
| `created_at`, `updated_at` | `TIMESTAMPTZ NOT NULL` | По умолчанию `NOW()`; `updated_at` меняют методы обновления профиля/отключения. |

### `teacher_students` — отношение преподавателя и ученика

| Поле | Тип | Назначение и ограничение |
|---|---|---|
| `teacher_id` | `BIGINT FK → users.id` | ID преподавателя; проверка роли/активности выполняется Go/SQL-операцией связывания. |
| `student_id` | `BIGINT FK → users.id` | ID ученика. `teacher_id <> student_id`. |
| `status` | `TEXT NOT NULL` | `active` (по умолчанию) / `removed`. |
| `joined_at` | `TIMESTAMPTZ NOT NULL` | Дата подключения; обновляется при повторном подключении. |

Первичный ключ **(`teacher_id`, `student_id`)** исключает повтор одной пары; индекс на `student_id` ускоряет обратный поиск. У одного ученика может быть несколько преподавателей. Связь `removed` остаётся в таблице, но не показывается в активных списках.

### `groups` — группы конкретного преподавателя

| Поле | Тип | Назначение и ограничение |
|---|---|---|
| `id` | `BIGSERIAL PK` | ID группы. |
| `teacher_id` | `BIGINT FK → users.id` | Владелец-преподаватель. |
| `name` | `TEXT NOT NULL` | Название, не может состоять только из пробелов. |
| `subject` | `TEXT NULL` | Предмет, необязателен; пустой после обрезки пробелов в Go превращается в `NULL`. |
| `status` | `TEXT NOT NULL` | `active` (по умолчанию) / `archived`. |
| `created_at` | `TIMESTAMPTZ NOT NULL` | Дата создания. |

Индекс на `teacher_id`. **Архивация — не удаление**: строка группы остаётся; архивные группы не возвращаются `ListTeacherGroups`.

### `group_members` — состав групп

| Поле | Тип | Назначение и ограничение |
|---|---|---|
| `group_id` | `BIGINT FK → groups.id` | Группа. |
| `student_id` | `BIGINT FK → users.id` | Ученик. |
| `joined_at` | `TIMESTAMPTZ NOT NULL` | Дата добавления. |

Первичный ключ **(`group_id`, `student_id`)** запрещает дубли; индекс на `student_id`. Участие не имеет отдельного статуса: `RemoveGroupMember` **удаляет строку**. При отключении связи преподаватель–ученик участие в его группах также удаляется.

### `invitations` — одноразовые приглашения

| Поле | Тип | Назначение и ограничение |
|---|---|---|
| `id` | `BIGSERIAL PK` | ID приглашения. |
| `teacher_id` | `BIGINT FK → users.id` | Создатель — преподаватель. |
| `group_id` | `BIGINT NULL FK → groups.id` | Необязательное присоединение к группе этого преподавателя. |
| `invitee_email` | `TEXT NULL` | Приглашение конкретному адресу или любому авторизованному ученику; при наличии — нижний регистр. |
| `token_hash` | `TEXT NOT NULL UNIQUE` | **SHA-256 в lowercase hex**, строго 64 символа; сырой токен в таблицу не записывается. |
| `status` | `TEXT NOT NULL` | `pending` (по умолчанию), `accepted`, `revoked`. Просрочка — проверка по времени, а не отдельный статус. |
| `expires_at` | `TIMESTAMPTZ NOT NULL` | Срок истечения приглашения. |
| `accepted_by` | `BIGINT NULL FK → users.id` | ID принявшего ученика, только для `accepted`. |
| `accepted_at` | `TIMESTAMPTZ NULL` | Дата принятия, только для `accepted`. |
| `created_at` | `TIMESTAMPTZ NOT NULL` | Дата создания. |

`CHECK` требует, чтобы `accepted_by` и `accepted_at` были заполнены **оба** только у `accepted`, иначе оба `NULL`. Индексы на `teacher_id` и непустой `group_id`. Уникальный хеш запрещает создавать два приглашения с одним токеном.

## 3. Структуры Go и их поля

Ниже **точные экспортируемые типы** из `backend/internal/db/user_models.go` и `group_models.go`. Внутренние SQL-сканеры возвращают именно эти структуры; у них **нет `json`-тегов**, и они не являются готовыми ответами frontend.

```go
// Публичный профиль, НЕТ password_hash.
type User struct {
    ID        int64     // users.id
    Email     string    // нормализованный email
    Name      string    // имя
    Role      string    // teacher/student/admin (роль из записи)
    Grade     *int16    // класс 1..12; nil = NULL
    Status    string    // active/disabled
    CreatedAt time.Time // время создания
    UpdatedAt time.Time // время изменения
}

// Данные для СОЗДАНИЯ учётной записи; не HTTP request напрямую.
type NewUser struct {
    Email        string // email, регистр/пробелы нормализуются
    PasswordHash string // готовый хеш, НЕ исходный пароль
    Name         string // имя
    Role         string // ТОЛЬКО teacher или student
    Grade        *int16 // 1..12 только для student, иначе nil
}

// СЕКРЕТНЫЕ внутренние данные для входа: нельзя сериализовать клиенту.
type UserCredentials struct {
    ID           int64
    Email        string
    PasswordHash string // auth-сервис сравнивает с введённым паролем
    Role         string
    Status       string
}

// Группа в базе.
type Group struct {
    ID        int64
    TeacherID int64
    Name      string
    Subject   *string // nil = NULL
    Status    string  // active/archived
    CreatedAt time.Time
}

// Входные данные создания группы.
type NewGroup struct {
    TeacherID int64   // подтверждённый ID преподавателя
    Name      string  // непустое после TrimSpace
    Subject   *string // nil или непустая строка; пустая => nil
}

// Входные данные создания приглашения.
type NewInvitation struct {
    TeacherID    int64     // активный преподаватель
    GroupID      *int64    // nil или ID ЕГО активной группы
    InviteeEmail *string   // nil = любому ученику, иначе валидный email
    TokenHash    string    // 64 символа lowercase SHA-256 hex
    ExpiresAt    time.Time // строго в будущем на момент создания
}

// Приглашение на чтение, БЕЗ token_hash.
type Invitation struct {
    ID           int64
    TeacherID    int64
    GroupID      *int64
    InviteeEmail *string
    Status       string     // pending/accepted/revoked
    ExpiresAt    time.Time
    AcceptedBy   *int64     // nil до принятия
    AcceptedAt   *time.Time // nil до принятия
    CreatedAt    time.Time
}
```

**Примеры необязательных полей:** `NewGroup{TeacherID: 42, Name: "ЕГЭ", Subject: nil}` — группа без предмета; `NewInvitation{GroupID: nil, InviteeEmail: nil, ...}` — приглашение без привязки к группе или адресу. `*int16`/`*int64` — указатели на число; для задания класса: `grade := int16(11); input.Grade = &grade`.

## 4. Все методы: параметры, поведение, ошибки

**Как читать:** `ErrValidation`, `ErrNotFound`, `ErrConflict` проверяются через `errors.Is`. Кроме перечисленного, **каждый метод может вернуть другую ошибку** при недоступности PostgreSQL, ошибке выполнения SQL, отмене контекста и т. п. Для `[]User`/`[]Group` «ничего не найдено» означает **пустой срез без ошибки**, а не `ErrNotFound`.

### Пользователи (`users.go`) — 5 методов

**`CreateUser(ctx context.Context, input NewUser) (*User, error)`**  
Принимает `NewUser` (все поля и правила в §3). Обрезает пробелы email/имени, email приводит к нижнему регистру и проверяет `net/mail`. Требует непустые имя и `PasswordHash`, роль только `teacher/student`, класс только ученику и только `1..12`. Создаёт строку `users` и возвращает **`*User` без пароля** с новым ID, `status=active`, временем. **Ошибки:** `ErrValidation` при нарушении перечисленных условий; `ErrConflict` при повторном email (без учёта регистра); прочие ошибки БД.

**`GetUser(ctx context.Context, id int64) (*User, error)`**  
`id` — ID записи `users`. Возвращает профиль, **включая `disabled`** (не проверяет статус или права вызывающего). **Ошибки:** `ErrNotFound`, если ID отсутствует; другие ошибки БД. Для `id<=0` отдельной проверки нет — обычно `ErrNotFound`.

**`GetUserByEmail(ctx context.Context, value string) (*UserCredentials, error)`**  
`value` — email; нормализуется и проверяется формат. Возвращает внутреннюю структуру **с `PasswordHash`**, ролью и статусом, даже если пользователь отключён. **Ошибки:** `ErrValidation` при некорректном email; `ErrNotFound` если адреса нет; ошибки БД. **Только для auth-сервиса: не отдавать результат в JSON.**

**`UpdateUserProfile(ctx context.Context, id int64, name string, grade *int16) (*User, error)`**  
`id` — кого изменяем; `name` — новое непустое имя (обрезаются пробелы); `grade` — новый класс `1..12`, `nil` сбрасывает класс в `NULL`. Преподавателю разрешено обновлять имя только с `grade=nil`; пароли и роль не меняются. Возвращает обновлённый `*User`, обновляет `updated_at`. **Ошибки:** `ErrValidation` для пустого имени/класса вне `1..12`; **`ErrNotFound` и при отсутствующем ID, и при попытке передать преподавателю ненулевой `grade`** (таково текущее SQL-поведение). Нет запрета обновлять `disabled` пользователя.

**`DisableUser(ctx context.Context, id int64) error`**  
`id` — кого отключить. Устанавливает `status=disabled`, `updated_at=NOW()`, **не удаляет запись и внешние связи**. Повторное отключение существующего пользователя возвращает `nil` (идемпотентно по состоянию). **Ошибка:** `ErrNotFound` если записи нет; другая ошибка БД. Сам по себе метод не отзывает активные сессии.

### Связи (`teacher_students.go`) — 4 метода

**`LinkTeacherStudent(ctx context.Context, teacherID, studentID int64) error`**  
`teacherID` — ID активного пользователя с ролью `teacher`; `studentID` — ID активного `student`. SQL `INSERT ... SELECT` создаёт связь со статусом `active`; `ON CONFLICT` **восстанавливает** ранее `removed`, обновляя `joined_at`. Повторная активная связь также допускается и обновляет `joined_at`. **Ошибка:** `ErrValidation` если любой пользователь отсутствует, отключён, имеет неверную роль или IDs не образуют допустимую пару; ошибки БД. **Не восстанавливает членство в старых группах.**

**`UnlinkTeacherStudent(ctx context.Context, teacherID, studentID int64) error`**  
Отключает **активную** связь (`status=removed`) и **в той же транзакции** удаляет ученика из `group_members` всех групп этого преподавателя. **Ошибка:** `ErrNotFound` если активной связи нет (включая уже `removed`); ошибки транзакции/БД. При ошибке до `COMMIT` изменение связи и чистка групп откатываются вместе.

**`ListTeacherStudents(ctx context.Context, teacherID int64) ([]User, error)`**  
Возвращает профили учеников по `teacher_students` с **активной связью и активным учеником**, сортировка по имени и ID. `teacherID` — ID преподавателя, чьих учеников ищем. Если связей нет / ID не найден — `[]User{}` без ошибки. **Не проверяет статус самого преподавателя и личность вызывающего**. Ошибки — только при SQL/чтении строк.

**`ListStudentTeachers(ctx context.Context, studentID int64) ([]User, error)`**  
Обратная выборка: преподаватели с активной связью и `users.status=active`, сортировка по имени и ID. Если нет связей / ID отсутствует — пустой срез. **Не проверяет статус самого ученика и права вызывающего**. Ошибки — при SQL/чтении строк.

### Группы (`groups.go`) — 8 методов

**`CreateGroup(ctx context.Context, input NewGroup) (*Group, error)`**  
`input.TeacherID` — положительный ID активного `teacher`; `input.Name` — название после `TrimSpace` непустое; `input.Subject` — предмет или `nil`, пробелы обрезаются, пустая строка превращается в `nil`. SQL создаёт группу только если владелец — активный преподаватель. Возвращает `*Group` с `status=active`, новым ID и временем. **Ошибки:** `ErrValidation` при пустом имени/неположительном ID/отсутствующем или неактивном преподавателе; ошибки БД.

**`GetGroup(ctx context.Context, groupID int64) (*Group, error)`**  
`groupID` — ID группы; возвращает запись **включая архивную**, без проверки владельца/прав вызывающего. **Ошибка:** `ErrNotFound` если группы нет; ошибки БД.

**`UpdateGroup(ctx context.Context, groupID, teacherID int64, name string, subject *string) (*Group, error)`**  
`groupID` — изменяемая группа; `teacherID` — ID **владельца**, `name`/`subject` — новые значения с правилами `CreateGroup`. Обновляет **только активную группу данного преподавателя** и возвращает её. **Ошибки:** `ErrValidation` для пустого названия; `ErrNotFound` если группа отсутствует, архивна или принадлежит другому преподавателю; ошибки БД. При `subject=nil` поле очищается (`NULL`).

**`ListTeacherGroups(ctx context.Context, teacherID int64) ([]Group, error)`**  
`teacherID` — ID владельца. Возвращает **только активные** группы, сортировка по ID; для пустого результата `[]Group{}`, без `ErrNotFound`. **Архивные группы не выдаются.** Не проверяет активность/роль переданного преподавателя и права инициатора.

**`ArchiveGroup(ctx context.Context, groupID, teacherID int64) error`**  
Помечает группу данного преподавателя `archived`, **не удаляя** её и записи `group_members`. **Ошибка:** `ErrNotFound` если ID не существует, группа чужая или уже архивна; ошибки БД. Не является идемпотентным вызовом: повторное архивирование даёт `ErrNotFound`.

**`AddGroupMember(ctx context.Context, groupID, studentID int64) error`**  
`groupID` — **активная группа** с активным преподавателем-владельцем; `studentID` — активный ученик, имеющий активную связь `teacher_students` **с владельцем именно этой группы**. Создаёт строку `group_members`. Повторное добавление допустимо: `nil`, прежний `joined_at` сохраняется. **Ошибка:** `ErrValidation` при отсутствии любого из условий (в том числе нет связи), ошибки БД. **Не проверяет полномочия HTTP-инициатора**.

**`RemoveGroupMember(ctx context.Context, groupID, studentID int64) error`**  
Удаляет **строку участия**, не аккаунт. **Ошибка:** `ErrNotFound`, если пары нет; ошибки БД. SQL **не проверяет активность группы и её владельца**, а также личность инициатора — сервер должен проверить доступ самостоятельно.

**`ListGroupMembers(ctx context.Context, groupID int64) ([]User, error)`**  
Возвращает профили **активных участников** (при использовании штатных методов это ученики), которые входят в указанную **активную** группу, имеют активную связь с её **активным** преподавателем. Сортировка по имени и ID. Для несуществующей/архивной группы или отсутствия видимых участников возвращает **пустой срез**, не `ErrNotFound`. **Не проверяет, кому разрешено просматривать группу**. Сам запрос не проверяет `u.role='student'`: эту роль обеспечивает штатный метод добавления, но не голый внешний ключ.

### Приглашения (`invitations.go`) — 4 метода

**`CreateInvitation(ctx context.Context, input NewInvitation) (*Invitation, error)`**  
`input.TeacherID` — положительный ID активного преподавателя. `GroupID` необязателен, но если задан — положительный ID **активной группы этого преподавателя**. `InviteeEmail` — `nil` (любому активному ученику) или валидный email, приводится к нижнему регистру; `TokenHash` — **64 символа lowercase hex**, полученные как SHA-256 от криптографически случайного сырого токена; `ExpiresAt` — **строго в будущем**. Возвращает `*Invitation` с `status=pending`, временем и ID, **без токена/хеша**. **Ошибки:** `ErrValidation` для неверных полей/владельца/группы; `ErrConflict` при дублировании хеша; ошибки БД. **Токен генерирует и доставляет сервер, не хранилище.**

**`GetInvitationByTokenHash(ctx context.Context, tokenHash string) (*Invitation, error)`**  
Ищет по готовому lowercase SHA-256 hex-хешу; возвращает `*Invitation` без хеша, **даже если статус `revoked`/`accepted` или срок истёк**. Это **не проверка действительности ссылки**. **Ошибки:** `ErrValidation` при неверном формате хеша; `ErrNotFound` если хеш отсутствует; ошибки БД. Вызов должен оставаться внутренним.

**`RevokeInvitation(ctx context.Context, invitationID, teacherID int64) error`**  
`invitationID` — ID приглашения, `teacherID` — его владелец. Меняет `pending → revoked`. **Ошибка:** `ErrNotFound` если не найдено, не принадлежит учителю либо уже `accepted/revoked`; ошибки БД. Не проверяет, что сам преподаватель сейчас `active`; эту проверку обеспечивает сервер. Просроченное, но ещё `pending` приглашение можно отозвать.

**`AcceptInvitation(ctx context.Context, tokenHash string, studentID int64) error`**  
`tokenHash` — корректный lowercase SHA-256 hex-хеш; `studentID` — **положительный ID реально авторизованного ученика**, который вызывает принятие. В транзакции `SELECT ... FOR UPDATE` блокирует приглашение, проверяет `status=pending`, срок, активную роль ученика, адрес `InviteeEmail` (если задан), активность преподавателя и принадлежность/активность группы. Затем **атомарно**: (1) создаёт/восстанавливает связь `teacher_students`, (2) добавляет в группу, если указана (`DO NOTHING` при повторном участии), (3) отмечает приглашение `accepted` и заполняет `accepted_by/accepted_at`, (4) `COMMIT`. **Ошибки:** `ErrValidation` при неверном хеше/ID, просрочке, несоответствии email, роли/статуса ученика или преподавателя, неверной группе; `ErrNotFound` если хеш отсутствует; `ErrConflict` если приглашение уже принято либо отозвано; ошибки БД/транзакции. **Повторный вызов не считается успехом**. Конкурентное принятие одного токена допускает только один успешный запрос; второй получает `ErrConflict`.

## 5. Общие ошибки и ограничения безопасности

| Значение Go | Когда возникает | Что делает сервер |
|---|---|---|
| `db.ErrValidation` | Неправильные значения полей, неверная роль/активность, просроченное приглашение и т. п. | Ошибка входных данных/бизнес-условия; HTTP-код выбирает сервер. |
| `db.ErrNotFound` | Нет записи **или операция не смогла изменить запись из-за статуса/владельца**. | Не выдавать лишние сведения о чужом объекте. |
| `db.ErrConflict` | Повтор email/хеша, уже использованное/отозванное приглашение. | Конфликт состояния/уникальности. |
| Иная `error` | Ошибки PostgreSQL, контекста, соединения, транзакции. | Внутренняя ошибка; не раскрывать детали БД клиенту. |

```go
switch {
case errors.Is(err, db.ErrValidation): // валидация/недопустимое состояние
case errors.Is(err, db.ErrNotFound):   // отсутствует или недоступно по условиям
case errors.Is(err, db.ErrConflict):   // дубликат/уже использовано
case err != nil:                       // неожиданная ошибка
}
```

**Обязательные обязанности auth/HTTP-сервера:**

- Проверять, кто вошёл, его `role`, `status` и владение/доступ **до** вызова методов, особенно `GetUser`, `GetGroup`, `ListGroupMembers`, `RemoveGroupMember`, `ListTeacherStudents`, `ListStudentTeachers`, `RevokeInvitation`.
- Никогда не сериализовать `UserCredentials.PasswordHash`, `NewUser.PasswordHash`, исходные токены/хеши приглашений. `UserCredentials` не имеет защитных `json`-тегов: **его прямой `json.Marshal` раскроет `PasswordHash`**. Использовать отдельные публичные DTO.
- Хешировать/проверять пароли стойким алгоритмом на сервере. Текущий `CreateUser` проверяет **только непустоту** готового хеша; он не обеспечивает хеширование.
- Создавать случайный сырой токен криптографическим генератором, передавать пользователю **только сырой токен по защищённому каналу**, а в БД — только SHA-256-хеш. Проверка формата не проверяет качество случайности.
- Реализовать управление сессиями и отзыв сессий после `DisableUser`: БД делает только `status=disabled`. Не принимать `studentID/teacherID` из тела запроса без проверки.
- Понимать, что `GetInvitationByTokenHash` не проверяет expiry, а методы `GetGroup`/`GetUser` допускают неактивные записи; сервер решает, что разрешено показывать.

## 6. Подключение, тесты и открытые задачи

### Подключение из Go-сервера

```go
import db "github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"

store, err := db.New(ctx, dsn) // PostgreSQL DSN берётся из окружения
if err != nil { return err }
defer store.Close()

// ПОСЛЕ аутентификации/проверки прав:
students, err := store.ListTeacherStudents(ctx, authenticatedTeacherID)
group, err := store.CreateGroup(ctx, db.NewGroup{
    TeacherID: authenticatedTeacherID,
    Name: "ЕГЭ информатика",
})
_ = students
_ = group
_ = err
```

`db.New(ctx, dsn)` создаёт `pgxpool` (до 10 соединений, срок жизни соединения до 1 часа, `Ping` при запуске). `db.Close()` закрывает пул. Создавать **один пул на приложение**, не на каждый HTTP-запрос. Миграции запускаются **до** использования методов.

### Проверки

Для интеграционных тестов нужна **отдельная** БД `edtech_test_db`, содержащая таблицы миграций `000004–000006`.

```bash
cd backend
export EDTECH_TEST_DSN='postgres://USER:LOCAL_PASSWORD@127.0.0.1:15432/edtech_test_db?sslmode=disable'
go test ./internal/db -run Integration -v -count=1
go test ./internal/db -v -count=1
go vet ./internal/db
```

`export` приведён для Git Bash; пароли не сохранять в Git. Если переменная не задана, интеграционные тесты пропускаются (`SKIP`). В логах разработчика от 10.11.2026 прошли **4 интеграционных и 4 модульных теста**; после переноса пакета в `internal/db` повторный запуск следует выполнить в актуальной ветке. Тесты используют фиктивные аккаунты и очищают свои записи.

