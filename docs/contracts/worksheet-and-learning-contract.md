
# Контракт сервисного слоя

**Пакеты:**
- `github.com/VIACHMEL/GO-Edtech-project/backend/internal/worksheetdata`
- `github.com/VIACHMEL/GO-Edtech-project/backend/internal/learningdata`

**Версия контракта:** 1.0

## 1. Назначение

Пакеты `worksheetdata` и `learningdata` образуют сервисный слой между HTTP-обработчиками и пакетом хранения `db`. Они не выполняют SQL-запросов, не проверяют права доступа и не работают с транзакциями — всё это уже реализовано в `db` (см. `db-backend-contract.md`).

### Задачи сервисного слоя

- Принимать запросы от HTTP-слоя в виде DTO (`*Request`).
- Передавать данные в методы `db`.
- Преобразовывать внутренние модели `db.*` в DTO (`*DTO`) для отправки клиенту.
- Оборачивать результаты составных операций (например, «заголовок варианта + список задач») в единый ответ.

### Что слой не делает

- Не проверяет роль и активность пользователя. Эти проверки встроены в SQL-запросы `db` (атомарно, без race condition).
- Не выполняет запросов к PostgreSQL напрямую.
- Не знает о существовании HTTP-протокола.

## 2. Общая схема взаимодействия

```text
[Браузер]
 │ HTTP/JSON
 ▼
[HTTP-обработчик] ← userID из доверенного контекста
 │ Request DTO
 ▼
[Service] ← ваш слой (worksheetdata / learningdata)
 │ вызов метода
 ▼
[db] ← проверки прав, транзакции, SQL
 │ внутренняя модель *db.X
 ▼
[Mapper] → Response DTO
 │
 ▼
[HTTP-обработчик]
 │ Response DTO
 ▼
[Браузер]
```

## 3. Пакет worksheetdata

### 3.1. Структуры (Response DTO)

#### 3.1.1. WorksheetDTO

Краткая информация о варианте. Используется в списках и как базовая часть детального ответа.

```go
type WorksheetDTO struct {
    ID int64 `json:"id"`
    Title string `json:"title"`
    Subject string `json:"subject"`
    MaxAttempts int `json:"max_attempts"`
    Status string `json:"status"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
    PublishedAt *time.Time `json:"published_at"`
}
```

| Поле | Тип | Описание |
|---|---|---|
| ID | int64 | Уникальный ID варианта |
| Title | string | Название варианта |
| Subject | string | Код предмета |
| MaxAttempts | int | Лимит попыток |
| Status | string | `draft`, `published`, `archived` |
| CreatedAt | time.Time | Дата создания |
| UpdatedAt | time.Time | Дата последнего изменения |
| PublishedAt | *time.Time | Дата публикации или null для черновика |

#### 3.1.2. WorksheetItemDTO

Позиция (задача) внутри варианта.

```go
type WorksheetItemDTO struct {
    ID int64 `json:"id"`
    TaskVersionID int64 `json:"task_version_id"`
    Position int `json:"position"`
    MaxScore string `json:"max_score"`
    Settings map[string]any `json:"settings"`
}
```

| Поле | Тип | Описание |
|---|---|---|
| ID | int64 | ID позиции в варианте |
| TaskVersionID | int64 | ID версии задачи из банка |
| Position | int | Порядковый номер (с 1) |
| MaxScore | string | Десятичная строка, например `"2.50"` (без потери точности NUMERIC) |
| Settings | map[string]any | Дополнительные настройки ({} если не заданы) |

#### 3.1.3. WorksheetDetailDTO

Детальная информация о варианте: заголовок + список позиций.

```go
type WorksheetDetailDTO struct {
    WorksheetDTO
    Items []WorksheetItemDTO `json:"items"`
}
```

#### 3.1.4. WorksheetListResponse

Ответ для эндпоинта со списком вариантов.

```go
type WorksheetListResponse struct {
    Worksheets []WorksheetDTO `json:"worksheets"`
    Total int `json:"total"`
}
```

### 3.2. Структуры запросов (Request DTO)

| Структура | Назначение |
|---|---|
| CreateWorksheetRequest | Создание варианта: title, subject, max_attempts |
| UpdateWorksheetRequest | Изменение черновика: title, subject, max_attempts |
| AddWorksheetItemRequest | Добавление позиции: task_version_id, position, max_score, settings |
| UpdateWorksheetItemRequest | Изменение позиции: max_score, settings |
| ReorderWorksheetItemsRequest | Перестановка: item_ids []int64 |
| CloneWorksheetRequest | Клонирование: title |

### 3.3. Методы Service

#### 3.3.1. CreateWorksheet

```go
func (s *Service) CreateWorksheet(ctx context.Context, teacherID int64, req CreateWorksheetRequest,) (WorksheetDTO, error)
```

Создаёт новый вариант в статусе `draft`. Хранилище проверяет, что `teacherID` — активный преподаватель.

**Возвращает:**
- `WorksheetDTO` при успехе;
- `db.ErrNotFound` — если преподаватель не найден/заблокирован
- `db.ErrValidation` — если невалидны title, subject или max_attempts.

#### 3.3.2. GetWorksheet

```go
func (s *Service) GetWorksheet(ctx context.Context, teacherID, id int64,) (WorksheetDetailDTO, error)
```

Возвращает вариант вместе со всеми позициями. Выполняет два запроса к хранилищу: заголовок и позиции.

**Возвращает:** `db.ErrNotFound` — если вариант не принадлежит этому преподавателю или не существует.

#### 3.3.3. ListWorksheets

```go
func (s *Service) ListWorksheets(ctx context.Context, teacherID, beforeID int64, limit int,) (WorksheetListResponse, error)
```

Список вариантов преподавателя, от новых к старым. `beforeID = 0` — первая страница. Лимит по умолчанию — 20, максимум — 100.

#### 3.3.4. UpdateWorksheet

```go
func (s *Service) UpdateWorksheet(ctx context.Context, teacherID, id int64, req UpdateWorksheetRequest,) error
```

Изменяет черновик. Если вариант не `draft` — `db.ErrWorksheetLocked`. Если новый предмет не совпадает с предметом выбранных задач — `db.ErrValidation`.

#### 3.3.5. PublishWorksheet

```go
func (s *Service) PublishWorksheet(ctx context.Context, teacherID, id int64,) (WorksheetDTO, error)
```

Публикует вариант. Хранилище возвращает только error, поэтому сервис перечитывает обновлённый вариант через `GetWorksheet`, чтобы отдать фронту актуальный статус.

Пустой вариант опубликовать нельзя — `db.ErrValidation`.

#### 3.3.6. ArchiveWorksheet

```go
func (s *Service) ArchiveWorksheet(ctx context.Context, teacherID, id int64,) error
```

Перемещает вариант в архив. Повторный вызов идемпотентен.

#### 3.3.7. CloneWorksheet

```go
func (s *Service) CloneWorksheet(ctx context.Context, teacherID, id int64, req CloneWorksheetRequest,) (WorksheetDTO, error)
```

Создаёт полную копию варианта (включая все позиции) в виде нового черновика.

#### 3.3.8. AddWorksheetItem

```go
func (s *Service) AddWorksheetItem(ctx context.Context, teacherID, id int64, req AddWorksheetItemRequest,) (WorksheetItemDTO, error)
```

Добавляет позицию в черновик. Хранилище проверяет, что предмет задачи из `task_versions` совпадает с предметом варианта. Одна версия задачи может быть добавлена несколько раз на разные позиции.

**Возвращает:**
- `db.ErrWorksheetConflict` — если позиция уже занята;
- `db.ErrValidation` — невалидный балл или предмет.

#### 3.3.9. UpdateWorksheetItem

```go
func (s *Service) UpdateWorksheetItem(ctx context.Context, teacherID, id, itemID int64, req UpdateWorksheetItemRequest,) error
```

Меняет балл и настройки позиции в черновике.

#### 3.3.10. RemoveWorksheetItem

```go
func (s *Service) RemoveWorksheetItem(ctx context.Context, teacherID, id, itemID int64,) error
```

Удаляет позицию. Позиции не уплотняются: после удаления могут остаться «дырки» (1, 2, 4). Для уплотнения используйте `ReorderWorksheetItems`.

#### 3.3.11. ReorderWorksheetItems

```go
func (s *Service) ReorderWorksheetItems(ctx context.Context, teacherID, id int64, req ReorderWorksheetItemsRequest,) error
```

Переставляет позиции. `ItemIDs` должен содержать ровно все ID позиций варианта без дублей и пропусков. Порядок в массиве — новый порядок позиций.

### 3.4. Функции-мапперы

| Функция | Вход | Выход |
|---|---|---|
| ToWorksheetDTO | *db.Worksheet | WorksheetDTO |
| ToWorksheetItemDTO | *db.WorksheetItem | WorksheetItemDTO |
| ToWorksheetDetailDTO | *db.Worksheet, []*db.WorksheetItem | WorksheetDetailDTO |

Маппер `ToWorksheetItemDTO` гарантирует, что поле `Settings` в JSON будет `{}`, а не `null`.

## 4. Пакет learningdata

### 4.1. Структуры (Response DTO)

#### 4.1.1. AssignmentDTO

Информация о назначении варианта ученику.

```go
type AssignmentDTO struct {
    ID int64 `json:"id"`
    WorksheetID int64 `json:"worksheet_id"`
    StudentID int64 `json:"student_id"`
    AssignedBy int64 `json:"assigned_by"`
    GroupID *int64 `json:"group_id"`
    DistributionID string `json:"distribution_id"`
    DueAt *time.Time `json:"due_at"`
    Status string `json:"status"`
    AssignedAt time.Time `json:"assigned_at"`
    MaxAttempts int `json:"max_attempts"`
}
```

| Поле | Описание |
|---|---|
| ID | ID назначения |
| WorksheetID | ID выданного варианта |
| StudentID | ID ученика |
| AssignedBy | ID преподавателя, выдавшего работу |
| GroupID | ID группы или null, если выдано лично |
| DistributionID | UUID операции выдачи (для идемпотентности) |
| DueAt | Дедлайн или null |
| Status | `active`, `closed`, `cancelled` |
| AssignedAt | Время выдачи |
| MaxAttempts | Лимит попыток (копируется из варианта) |

#### 4.1.2. AttemptDTO

Конкретная попытка решения.

```go
type AttemptDTO struct {
    ID int64 `json:"id"`
    AssignmentID int64 `json:"assignment_id"`
    WorksheetID int64 `json:"worksheet_id"`
    AttemptNo int `json:"attempt_no"`
    Status string `json:"status"`
    StartedAt time.Time `json:"started_at"`
    SubmittedAt *time.Time `json:"submitted_at"`
    ResultPublishedAt *time.Time `json:"result_published_at"`
    IsLate bool `json:"is_late"`
}
```

| Поле | Описание |
|---|---|
| AttemptNo | Номер попытки (1, 2, 3…) |
| Status | `draft`, `submitted`, `reviewed` |
| SubmittedAt | Когда сдана, null если ещё черновик |
| ResultPublishedAt | Когда опубликована оценка |
| IsLate | Сдано ли после дедлайна (фиксируется при первой сдаче) |

#### 4.1.3. TeacherAnswerDTO

Ответ ученика в представлении преподавателя.

```go
type TeacherAnswerDTO struct {
    WorksheetItemID int64 `json:"worksheet_item_id"`
    Position int `json:"position"`
    Text string `json:"text"`
    Data map[string]any `json:"data"`
    AnswerRevision int `json:"answer_revision"`
    GradeStatus string `json:"grade_status"`
    GradedRevision *int `json:"graded_revision"`
    IsCorrect *bool `json:"is_correct"`
    Score *string `json:"score"`
    Comment string `json:"comment"`
    LiveState *int `json:"live_state"`
}
```

| Поле | Описание |
|---|---|
| GradeStatus | `pending`, `auto`, `manual` |
| IsCorrect, Score | null, пока ответ не проверен |
| Score | Строка (`"2.50"`) для точности NUMERIC |
| LiveState | -1 нет ответа, 0 неверно, 1 верно, null ждёт проверки |

#### 4.1.4. Ответы, которые не нуждаются в DTO

Модели `db.StudentAnswer` и `db.SavedAnswer` уже имеют JSON-теги в самом пакете `db`. Поэтому для них DTO не создаются — методы сервиса возвращают их напрямую.

### 4.2. Структуры запросов (Request DTO)

| Структура | Назначение |
|---|---|
| AssignWorksheetRequest | Выдача одному ученику: worksheet_id, student_id, distribution, due_at |
| AssignWorksheetToGroupRequest | Выдача группе: worksheet_id, group_id, distribution, due_at |
| UpdateStatusRequest | Смена статуса: status (active/closed/cancelled) |
| UpdateDeadlineRequest | Изменение срока: due_at (null — убрать срок) |
| SaveAnswerRequest | Сохранение ответа: text, data, expected_revision |
| ReviewAnswerRequest | Ручная оценка: revision, score, comment |
| AutoGradeRequest | Авто-проверка: revision |

### 4.3. Методы Service: преподаватель

#### 4.3.1. AssignWorksheet

```go
func (s *Service) AssignWorksheet(ctx context.Context, teacherID int64, req AssignWorksheetRequest,) (AssignmentDTO, error)
```

Выдаёт опубликованный вариант конкретному ученику. Поле `Distribution` (UUID) — ключ идемпотентности: повторный запрос с тем же UUID вернёт те же записи. Если UUID уже использован с другими параметрами (другой ученик/вариант/срок) — `db.ErrConflict`.

#### 4.3.2. AssignWorksheetToGroup

```go
func (s *Service) AssignWorksheetToGroup(ctx context.Context, teacherID int64, req AssignWorksheetToGroupRequest,) ([]AssignmentDTO, error)
```

Атомарно создаёт назначения для всех активных участников группы. Пустая группа → `db.ErrValidation`. Последующие изменения состава группы не влияют на уже созданные назначения.

#### 4.3.3. ListTeacherAssignments

```go
func (s *Service) ListTeacherAssignments(ctx context.Context, teacherID, beforeID int64, limit int,) ([]AssignmentDTO, error)
```

Список назначений преподавателя. Пагинация через `beforeID` (ID последнего элемента предыдущей страницы).

#### 4.3.4. SetAssignmentStatus

```go
func (s *Service) SetAssignmentStatus(ctx context.Context, teacherID, id int64, req UpdateStatusRequest,) error
```

Закрывает или отменяет назначение. Закрытое назначение нельзя редактировать и сдавать; ранее сданные ответы сохраняются.

#### 4.3.5. UpdateAssignmentDeadline

```go
func (s *Service) UpdateAssignmentDeadline(ctx context.Context, teacherID, id int64, req UpdateDeadlineRequest,) error
```

Меняет срок сдачи. `nil` в поле `DueAt` убирает срок. На ранее зафиксированный `is_late` первой сдачи не влияет.

#### 4.3.6. ReopenAttempt

```go
func (s *Service) ReopenAttempt(ctx context.Context, teacherID, assignmentID int64,) (AttemptDTO, error)
```

Создаёт новую попытку в пределах лимита. Если у ученика уже есть `draft` — возвращает его. Исчерпан лимит — `db.ErrAttemptLimit`.

#### 4.3.7. ListAssignmentAttempts

```go
func (s *Service) ListAssignmentAttempts(ctx context.Context, actorID, assignmentID int64,) ([]AttemptDTO, error)
```

Список всех попыток по назначению. Доступно владельцу-ученику и преподавателю назначения (проверка в `db`).

#### 4.3.8. GetAttempt

```go
func (s *Service) GetAttempt(ctx context.Context, actorID, id int64,) (AttemptDTO, error)
```

Конкретная попытка. `actorID` — либо ученик-владелец, либо преподаватель назначения.

#### 4.3.9. AutoGradeAnswer

```go
func (s *Service) AutoGradeAnswer(ctx context.Context, teacherID, attemptID, itemID int64, req AutoGradeRequest,) error
```

Автоматическая проверка краткого ответа по точному совпадению (после `trim`). Ручную оценку не перезаписывает. Revision должен совпадать с текущей — иначе `db.ErrRevisionConflict`.

#### 4.3.10. ReviewAnswer

```go
func (s *Service) ReviewAnswer(ctx context.Context, teacherID, attemptID, itemID int64, req ReviewAnswerRequest,) error
```

Ручная оценка. `Score` — строка вида `"2.50"`; может переопределить авто-оценку. Если `Score > max_score` позиции — `db.ErrValidation`. Комментарий до 5000 Unicode-символов.

#### 4.3.11. PublishAttemptResult

```go
func (s *Service) PublishAttemptResult(ctx context.Context, teacherID, attemptID int64,) error
```

Публикует оценку ученику. Все позиции должны иметь оценку актуальной ревизии — иначе `db.ErrIncompleteGrading`. Повторная публикация идемпотентна.

#### 4.3.12. GetTeacherLiveState

```go
func (s *Service) GetTeacherLiveState(ctx context.Context, teacherID, attemptID int64,) ([]TeacherAnswerDTO, error)
```

«Живое» состояние ответов для экрана проверки. Возвращает тексты, ревизии, статусы проверки и предварительный `LiveState`. Транспорт (SSE/WS) в этот метод не входит.

### 4.4. Методы Service: ученик

#### 4.4.1. ListStudentAssignments

```go
func (s *Service) ListStudentAssignments(ctx context.Context, studentID, beforeID int64, limit int,) ([]AssignmentDTO, error)
```

Список назначений ученика.

#### 4.4.2. StartOrResumeAttempt

```gofunc (s *Service) StartOrResumeAttempt(ctx context.Context, studentID, assignmentID int64,) (AttemptDTO, error)
```

Возвращает существующий `draft` либо создаёт следующую попытку в пределах `max_attempts`. Параллельные запросы возвращают один и тот же `draft`. Исчерпан лимит → `db.ErrAttemptLimit`. Назначение закрыто/отменено → `db.ErrWorkClosed`.

#### 4.4.3. GetAttemptAnswers

```gofunc (s *Service) GetAttemptAnswers(ctx context.Context, studentID, attemptID int64,) ([]*db.StudentAnswer, error)
```

Возвращает условия задач и сохранённые ответы ученика. Модель `db.StudentAnswer` уже содержит JSON-теги и отдаётся фронту без промежуточного DTO.

Условия берутся из снимка, зафиксированного при публикации варианта — последующие изменения банка задач на эту попытку не влияют.

До публикации результата поля `result` в ответе нет вообще. После — содержит `is_correct`, `score`, `comment`.

#### 4.4.4. SaveAnswer

```go
func (s *Service) SaveAnswer(ctx context.Context, studentID, attemptID, itemID int64, req SaveAnswerRequest,) (*db.SavedAnswer, error)
```

Сохраняет ответ на позицию. Требует `ExpectedRevision` — текущую ревизию, которую видит клиент. Если на сервере ревизия другая — `db.ErrRevisionConflict`. Повтор непосредственно предыдущего идентичного сохранения возвращает ту же ревизию (идемпотентно).

**Лимиты:**
- 10 000 Unicode-символов текста;
- 1 MiB сериализованного Data.

Попытка не в статусе `draft` → `db.ErrAttemptLocked`.

#### 4.4.5. ClearAnswer

```go
func (s *Service) ClearAnswer(ctx context.Context, studentID, attemptID, itemID int64, revision int,) (*db.SavedAnswer, error)
```

Очищает текст и Data ответа. Правила ревизий — как у `SaveAnswer`.

#### 4.4.6. SubmitAttempt

```go
func (s *Service) SubmitAttempt(ctx context.Context, studentID, attemptID int64,) (AttemptDTO, error)
```

Сдаёт работу. Запускает авто-проверку кратких ответов и фиксирует `submitted_at`, `is_late`. Повторная сдача не меняет зафиксированные значения.

### 4.5. Функции-мапперы

| Функция | Вход | Выход |
|---|---|---|
| ToAssignmentDTO | *db.Assignment | AssignmentDTO |
| ToAttemptDTO | *db.Attempt | AttemptDTO |
| ToTeacherAnswerDTO | *db.TeacherAnswer | TeacherAnswerDTO |

Для `db.StudentAnswer` и `db.SavedAnswer` мапперы не создаются — модели уже готовы к JSON-сериализации.

## 5. Ошибки

Сервисный слой не создаёт собственных ошибок и не оборачивает ошибки хранилища. Все ошибки пакета `db` пробрасываются наверх без изменений:

| Ошибка (errors.Is) | HTTP-код | Причина |
|---|---:|---|
| db.ErrValidation | 400 | Невалидные поля, превышение лимитов |
| db.ErrNotFound | 404 | Запись не найдена или принадлежит другому пользователю |
| db.ErrConflict | 409 | Конфликт UUID выдачи |
| db.ErrWorksheetLocked | 409 | Попытка редактировать опубликованный/архивный вариант |
| db.ErrWorksheetConflict | 409 | Занятая позиция в варианте |
| db.ErrAttemptLocked | 409 | Попытка редактировать сданную/проверенную попытку |
| db.ErrWorkClosed | 409 | Назначение закрыто или отменено |
| db.ErrRevisionConflict | 409 | Устаревшая ревизия ответа |
| db.ErrAttemptLimit | 429 | Исчерпан лимит попыток |
| db.ErrIncompleteGrading | 409 | Не все ответы оценены перед публикацией |
| Прочие | 500 | Ошибка БД |

**Правило для HTTP-слоя:** маппинг ошибок в HTTP-коды выполняется в одном месте (функция `mapErr` роутера), а не в каждом обработчике.

## 6. Правила заполнения Request-структур

| Поле | Обязательно | Правило |
|---|---|---|
| Title (в CreateWorksheetRequest) | да | 1–200 Unicode-символов |
| Subject | да | 1–64 символа, должен совпадать с предметом добавляемых задач |
| MaxAttempts | да | Целое 1–2147483647; 0 не означает «неограниченно» |
| TaskVersionID | да | Положительное число, существует в task_versions |
| Position | да | Положительное; уникальна в пределах варианта |
| MaxScore | да | Десятичная строка "0"–"9999.99", не более 2 знаков после точки |
| ItemIDs (в Reorder) | да | Содержит ровно все ID позиций варианта, без дублей |
| Distribution | да | Валидный UUID (нижний регистр, с дефисами) |
| ExpectedRevision | да | int ≥ 0; для нового ответа — 0 |
| Revision (в Review) | да | Текущая answer_revision ответа |
| Score (в Review) | да | Строка "0"–"9999.99", не более 2 знаков после точки; ≤ max_score позиции |
| Comment (в Review) | нет | До 5000 Unicode-символов |

## 7. Инициализация

Оба пакета имеют одинаковый конструктор:

```go
func NewService(store *db.DB) *Service
```

Пример подключения в `main.go` или роутере:

```go
store, err := db.New(ctx, dsn)
if err != nil {
 log.Fatal(err)
}
defer store.Close()

wsSvc := worksheetdata.NewService(store)
learnSvc := learningdata.NewService(store)

mux.Handle("POST /api/teacher/worksheets", createWorksheet(wsSvc))
// ...
```

## 8. Сводная таблица методов

### worksheetdata

| Метод | Назначение |
|---|---|
| CreateWorksheet | Создать черновик |
| GetWorksheet | Получить вариант + позиции |
| ListWorksheets | Список своих вариантов |
| UpdateWorksheet | Изменить черновик |
| PublishWorksheet | Опубликовать |
| ArchiveWorksheet | Архивировать |
| CloneWorksheet | Клонировать |
| AddWorksheetItem | Добавить позицию |
| UpdateWorksheetItem | Изменить позицию |
| RemoveWorksheetItem | Удалить позицию |
| ReorderWorksheetItems | Переставить |

### learningdata — преподаватель

| Метод | Назначение |
|---|---|
| AssignWorksheet | Выдать ученику |
| AssignWorksheetToGroup | Выдать группе |
| ListTeacherAssignments | Свои назначения |
| SetAssignmentStatus | Закрыть/отменить |
| UpdateAssignmentDeadline | Изменить срок |
| ReopenAttempt | Открыть новую попытку |
| ListAssignmentAttempts | Все попытки по назначению |
| GetAttempt | Конкретная попытка |
| AutoGradeAnswer | Авто-проверка |
| ReviewAnswer | Ручная оценка |
| PublishAttemptResult | Опубликовать результат |
| GetTeacherLiveState | Live-состояние ответов |

### learningdata — ученик

| Метод | Назначение |
|---|---|
| ListStudentAssignments | Свои назначения |
| StartOrResumeAttempt | Начать/продолжить попытку |
| GetAttemptAnswers | Условия + свои ответы |
| SaveAnswer | Сохранить ответ |
| ClearAnswer | Очистить ответ |
| SubmitAttempt | Сдать работу |
