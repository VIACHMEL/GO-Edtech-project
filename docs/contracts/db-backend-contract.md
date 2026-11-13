# Контракт для backend

Пакет: `github.com/VIACHMEL/GO-Edtech-project/backend/pkg/db`  
Версия контракта: 1.0

---

## 1. Назначение

Пакет `db` предоставляет интерфейс для взаимодействия с PostgreSQL и позволяет получать информацию о задачах, их версиях и связанных с ними артефактах.

Основные возможности:
- Получение одной задачи по ID.
- Получение нескольких задач по списку ID.
- Получение списка случайных задач с фильтрацией.
- Получение артефактов конкретной версии задачи.

---

## 2. Основные структуры

### 2.1. `db.Task`

Основная структура задачи, возвращаемая методами получения данных.

```go
type Task struct {
    SourceURL   string
    SourceSite  string
    Subject     string
    TaskNumber  int
    ContentText string
    Answer      string

    Year     int
    ExamType string
    TaskType string
    Solution string

    Metadata map[string]any

    ID               int64
    ContentHash      string
    CurrentVersionID int64
    VersionNumber    int
    CreatedAt        time.Time
    UpdatedAt        time.Time
}
```

| Поле | Тип | Описание |
|---|---|---|
| `ID` | `int64` | Уникальный идентификатор задачи |
| `SourceURL` | `string` | Ссылка на источник задачи |
| `SourceSite` | `string` | Источник задачи |
| `Subject` | `string` | Предмет |
| `TaskNumber` | `int` | Номер задания |
| `ContentText` | `string` | Текст актуальной версии задачи |
| `Answer` | `string` | Ответ |
| `Year` | `int` | Год |
| `ExamType` | `string` | Тип экзамена |
| `TaskType` | `string` | Подтип задания |
| `Solution` | `string` | Решение |
| `Metadata` | `map[string]any` | Дополнительные данные |
| `ContentHash` | `string` | Хеш содержимого |
| `CurrentVersionID` | `int64` | ID актуальной версии |
| `VersionNumber` | `int` | Номер актуальной версии |
| `CreatedAt` | `time.Time` | Дата создания |
| `UpdatedAt` | `time.Time` | Дата обновления |

Задача возвращается вместе с данными своей актуальной версии.

### 2.2. `db.TaskArtifact`

Структура артефакта, связанного с определённой версией задачи.

```go
type TaskArtifact struct {
    ID               int64
    TaskVersionID    int64
    Type             string
    ArtifactKey      string
    FilePath         string
    ExternalURL      string
    OriginalFilename string
    MimeType         string
    FileSize         int64
    Checksum         string
    StorageType      string
    Status           string
    ErrorMessage     string
    CreatedAt        time.Time
    UpdatedAt        time.Time
}
```

| Поле | Тип | Описание |
|---|---|---|
| `ID` | `int64` | ID артефакта |
| `TaskVersionID` | `int64` | ID версии задачи |
| `Type` | `string` | Тип артефакта |
| `ArtifactKey` | `string` | Уникальный ключ артефакта |
| `FilePath` | `string` | Путь к локальному файлу |
| `ExternalURL` | `string` | Внешняя ссылка |
| `OriginalFilename` | `string` | Исходное имя файла |
| `MimeType` | `string` | MIME-тип |
| `FileSize` | `int64` | Размер файла в байтах |
| `Checksum` | `string` | Контрольная сумма |
| `StorageType` | `string` | Тип хранилища |
| `Status` | `string` | Статус артефакта |
| `ErrorMessage` | `string` | Сообщение об ошибке |
| `CreatedAt` | `time.Time` | Дата создания |
| `UpdatedAt` | `time.Time` | Дата обновления |

### 2.3. `db.TaskFilter`

Структура параметров фильтрации для получения списка задач.

```go
type TaskFilter struct {
    Subject    string
    Year       int
    TaskNumber int
    TaskType   string
    Limit      int
}
```

| Поле | Тип | Описание |
|---|---|---|
| `Subject` | `string` | Предмет |
| `Year` | `int` | Год |
| `TaskNumber` | `int` | Номер задания |
| `TaskType` | `string` | Тип задания |
| `Limit` | `int` | Количество возвращаемых задач |

Пустые значения фильтров не ограничивают выборку по соответствующему полю.

Если `Limit` меньше 1 или больше 100, используется значение по умолчанию — 20.

---

## 3. Методы получения данных

### 3.1. GetTask

```go
func (db *DB) GetTask(ctx context.Context, id int64) (*Task, error)
```

Получает одну задачу по её идентификатору.

**Параметры:**
- `ctx` — контекст выполнения запроса.
- `id` — ID задачи.

**Возвращает:**
- `*Task` — найденная задача с данными актуальной версии.
- `ErrNotFound` — если задача не найдена.
- Другую ошибку — при проблемах с БД.

### 3.2. GetTasksByIDs

```go
func (db *DB) GetTasksByIDs(
    ctx context.Context,
    ids []int64,
) ([]*Task, error)
```

Получает несколько задач по списку идентификаторов.

**Параметры:**
- `ctx` — контекст выполнения запроса.
- `ids` — список ID задач.

**Возвращает:**
- `[]*Task` — найденные задачи с данными актуальных версий.
- `error` — ошибку при выполнении запроса.

**Особенности:**
- Возвращаются только существующие задачи.
- Порядок результатов не гарантируется.
- При пустом списке ID возвращается пустой срез без ошибки.

### 3.3. ListTasks

```go
func (db *DB) ListTasks(
    ctx context.Context,
    filter TaskFilter,
) ([]*Task, error)
```

Получает случайную выборку задач с возможностью фильтрации.

**Параметры:**
- `ctx` — контекст выполнения запроса.
- `filter` — параметры фильтрации.

**Доступные фильтры:**
- Предмет (`Subject`).
- Год (`Year`).
- Номер задания (`TaskNumber`).
- Тип задания (`TaskType`).
- Количество результатов (`Limit`).

**Возвращает:**
- `[]*Task` — список найденных задач.
- `error` — ошибку при выполнении запроса.

**Особенности:**
- Все заданные фильтры применяются одновременно.
- Результаты выбираются в случайном порядке.
- Максимальное количество результатов — 100.
- При некорректном `Limit` используется значение 20.
- Возвращаются данные актуальных версий задач.

### 3.4. GetArtifactsByVersionID

```go
func (db *DB) GetArtifactsByVersionID(
    ctx context.Context,
    versionID int64,
) ([]*TaskArtifact, error)
```

Получает все артефакты, связанные с определённой версией задачи.

**Параметры:**
- `ctx` — контекст выполнения запроса.
- `versionID` — ID версии задачи.

**Возвращает:**
- `[]*TaskArtifact` — список артефактов.
- `error` — ошибку при выполнении запроса.

**Особенности:**
- Возвращаются все артефакты указанной версии независимо от их статуса.
- Результаты сортируются по типу (`Type`) и ключу (`ArtifactKey`).
- Если артефактов нет, возвращается пустой срез.
- Для внешних ресурсов `FilePath` будет пустым.
- Для локальных файлов `ExternalURL` будет пустым.

---

## 4. Константы

### 4.1. Типы артефактов

```go
db.ArtifactTypeImage      = "image"
db.ArtifactTypeSVG        = "svg"
db.ArtifactTypeTikZ       = "tikz"
db.ArtifactTypeAttachment = "attachment"
db.ArtifactTypePreview    = "preview"
```

### 4.2. Статусы артефактов

```go
db.StatusPending   = "pending"
db.StatusDone      = "done"
db.StatusFailed    = "failed"
db.StatusAbandoned = "abandoned"
db.StatusExternal  = "external"
```

### 4.3. Типы хранилищ

```go
db.StorageLocal    = "local"
db.StorageS3       = "s3"
db.StorageExternal = "external"
```

---

## 5. Ошибки

Пакет предоставляет следующие ошибки:

```go
db.ErrValidation
db.ErrNotFound
```

| Ошибка | Описание |
|---|---|
| `ErrValidation` | Переданные данные не прошли проверку |
| `ErrNotFound` | Запрашиваемая запись не найдена |

Для проверки ошибок можно использовать `errors.Is()`.

---

## 6. Инициализация подключения

Для работы с БД используется структура `db.DB`.

```go
type DB struct {
    Pool *pgxpool.Pool
}
```

Создание подключения:

```go
func New(ctx context.Context, dsn string) (*DB, error)
```

**Параметры:**
- `ctx` — контекст выполнения.
- `dsn` — строка подключения к PostgreSQL.

**Возвращает:**
- `*DB` — экземпляр подключения к БД.
- `error` — ошибку подключения.

Параметры пула:
- Максимальное количество соединений — 10.
- Максимальное время жизни соединения — 1 час.
- При создании подключения выполняется проверка доступности БД (`Ping`).

Закрытие подключения:

```go
func (db *DB) Close()
```

Закрывает пул соединений.

---

## 7. Сводная таблица возможностей

| Метод | Назначение | Результат |
|---|---|---|
| `New` | Создание подключения к БД | `*DB` |
| `Close` | Закрытие подключения | — |
| `GetTask` | Получение задачи по ID | `*Task` |
| `GetTasksByIDs` | Получение задач по списку ID | `[]*Task` |
| `ListTasks` | Получение случайных задач с фильтрами | `[]*Task` |
| `GetArtifactsByVersionID` | Получение артефактов версии | `[]*TaskArtifact` |

**Примечание:** пакет предоставляет доступ к данным задач и артефактов. Получение истории версий, поиск по произвольным полям и постраничная навигация в текущий интерфейс пакета не входят.