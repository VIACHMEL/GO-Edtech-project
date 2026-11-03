# Контракт для парсеров

Пакет: `github.com/VIACHMEL/GO-Edtech-project/backend/pkg/db`  
Версия контракта: 2.0

---

## 1. Структуры

### 1.1. `db.Task` — входная структура для `AddTask` / `AddTasks`
Сама структура задачи, хранит всю инормацию о конкретной задаче
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

    // Заполняется БД, парсер не трогает:
    ID               int64
    ContentHash      string
    CurrentVersionID int64
    VersionNumber    int
    CreatedAt        time.Time
    UpdatedAt        time.Time
}
```
### 1.2. db.AddTaskResult — возврат из AddTask
нужна для проверки что задача добавилась
```go
type AddTaskResult struct {
    TaskID     int64
    VersionID  int64
    VersionNum int
    WasCreated bool
}
```
### 1.3. db.TaskArtifact — возврат из GetArtifactsByVersionID
структура хранит всю информацию о вложенном файле к задаче

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
## 2. Функции

### 2.1. AddTask
добавляет задачу в БД
```go
func (db *DB) AddTask(ctx context.Context, t *Task) (*AddTaskResult, error)
```
**Вход:**

- ctx — контекст с таймаутом.
- t — указатель на Task с заполненными полями.
**Возврат:**

*AddTaskResult с полями:

- TaskID — ID задачи в БД (один и тот же для всех версий).
- VersionID — ID версии; нужен для всех последующих вызовов артефактов.
- VersionNum — номер версии (1, 2, 3, ...)
- WasCreated — true если создана новая версия, false если обновлена существующая.
**error:**

- ErrValidation — невалидные поля задачи. Задача не сохранена.
- Прочие — ошибки БД.
**Поведение:**

- Если source_url уже существует и content_hash (Subject:TaskNumber:ContentText) совпадает — обновляется answer / solution существующей версии, WasCreated = false.
- Если source_url существует, но content_hash новый — создаётся новая версия с version_number = MAX+1, WasCreated = true.
- t мутируется (тримминг строковых полей). t.ID, t.VersionNumber, t.CurrentVersionID не заполняются — брать из возврата.
### 2.2. AddTasks
добавляет пакет задач в БД
```go
func (db *DB) AddTasks(ctx context.Context, tasks []*Task) error
```
**Вход:**

- ctx — контекст.
- tasks — срез указателей на Task.
**Возврат:**

- nil — все задачи сохранены (транзакция закоммичена).
- ErrValidation — хотя бы одна задача невалидна; откатились все.
- Прочие — ошибки БД
**Поведение:**

- Все задачи в одной транзакции. Ошибка на любой — откат всех.
- Не возвращает VersionID. Для получения версий после AddTasks используйте GetTask по SourceURL. Мб в будущем сделаю чтобы возвращался массив добавленых задач. (звучит на мой взгляд как так себе идея, но если надо сделаю)
- nil / пустой срез — no-op, nil.
### 2.3. CreatePendingArtifact
Говорит БД что у этой задачи есть какой-то вложенный файл
```go
func (db *DB) CreatePendingArtifact(ctx context.Context,
    versionID int64, typ, key, externalURL, originalFilename string) error
```
**Вход:**

- ctx — контекст.
- versionID — из AddTaskResult.VersionID.
- typ — тип артефакта, только из констант db.ArtifactType*.
- key — ключ артефакта, [a-zA-Z0-9_\-]{1,64}. "" → "main". По факту для случаев когда несколько картинок к 1-й задаче, чтобы порядок сделать, надо согласовать по хорошему неймниг с фронтом
- externalURL — URL источника; "" если файл рендерите сами или он уже у вас.
- originalFilename — оригинальное имя файла ("27_A.txt"); "" если неизвестно.
**Возврат:**

- nil — запись создана или обновлена (статус pending).
- ErrValidation — невалидный typ или key.
**Поведение:**

Создаёт строку со статусом pending.
- Если артефакт с таким (versionID, typ, key) уже есть:

external_url обновляется;
original_filename обновляется, если новый непустой;
status: если текущий done и checksum непустой — остаётся done; иначе pending;
- error_message очищается.
### 2.4. CompleteArtifact
Говорит БД что вы создали где-то в хранилище файл к задаче
```go
func (db *DB) CompleteArtifact(ctx context.Context,
    versionID int64, typ, key, filePath, mimeType string,
    fileSize int64, checksum string) error
```
**Вход:**

- ctx — контекст.
- versionID — из AddTaskResult.VersionID.
- typ, key — как в CreatePendingArtifact; должны совпадать с существующей записью.
- filePath — относительный путь от DATA_ROOT (без ведущего /). Непустой. (Вообще про путь можно отдельно согласовать, пока сделано так, если что изменим)
- mimeType — MIME, определённый по содержимому (не по заголовку источника).
- fileSize — размер файла в байтах.
- checksum — "sha256:<hex>" от содержимого. Непустой.
**Возврат:**

- nil — статус done, файл зафиксирован.
- ErrValidation — пустой filePath или checksum.
- ErrNotFound — записи с таким (versionID, typ, key) нет; забыли CreatePendingArtifact.
**Поведение:**

- storage_type = "local".
- error_message очищается.
Вызывать только после того, как файл физически оказался по filePath (atomic rename из tmp/).
### 2.5. MarkExternal
Говорит БД что к данной задаче в сети есть вложенный файл. Только для каких-то больших файлов типо видео
```go
func (db *DB) MarkExternal(ctx context.Context,
    versionID int64, typ, key, externalURL, mimeType string) error
```
**Вход:**

- ctx — контекст.
- versionID — из AddTaskResult.VersionID.
- typ, key — как в CreatePendingArtifact.
- externalURL — абсолютный URL, непустой.
- mimeType — MIME ресурса.
**Возврат:**

- nil — статус external, файла локально нет.
- ErrValidation — пустой externalURL, невалидный typ / key.
- ErrNotFound — записи с таким (versionID, typ, key) нет.
**Поведение:**

- storage_type = "external", file_path = NULL, status = "external".
Используется, когда файл решено не скачивать: видео, интерактив, большие ресурсы.
### 2.6. FailArtifact
Говорит БД что скачать или получить URl на файл не получилось по какой-то причине
```go
func (db *DB) FailArtifact(ctx context.Context,
    versionID int64, typ, key, errMsg string) error
```
**Вход:**

- ctx — контекст.
- versionID — из AddTaskResult.VersionID.
- typ, key — как в CreatePendingArtifact.
- errMsg — причина сбоя, 1–500 символов, без персональных данных.
**Возврат:**

- nil — статус failed, error_message сохранён.
- ErrValidation — невалидный key.
- ErrNotFound — записи с таким (versionID, typ, key) нет.
**Поведение:**

Строка остаётся в БД; подхватывается фоновой retry-джобой.
### 2.7. GetArtifactsByVersionID
Получает все существующие вложенные файлы к задаче. По факту для проверки
```go
func (db *DB) GetArtifactsByVersionID(ctx context.Context, versionID int64) ([]*TaskArtifact, error)
```
**Вход:**

- ctx — контекст.
- versionID — ID версии.
**Возврат:**

[]*TaskArtifact — все артефакты версии, отсортированы по (type, artifact_key).
Пустой срез [] если артефактов нет (не nil, не ошибка).
error — только ошибка БД.
**Поведение:**

- FilePath = "" для storage_type = "external".
- ExternalURL = "" для локальных файлов.
- ErrorMessage непустой только при status = "failed".
## 3. Правила заполнения полей

### 3.1. Task

| Поле | Обязательно | Тип | Правило | При отсутствии |
| --- | --- | --- | --- | --- |
| SourceURL | да | string | Абсолютный http(s)://, уникален | ErrValidation |
| SourceSite | да | string | Домен без схемы ("fipi.ru") | "" |
| Subject | да | string | Нормализованный ("math", не "Math") | ErrValidation |
| TaskNumber | да | int | >= 0 | ErrValidation |
| ContentText | да | string | Полный текст; крайние пробелы тримятся | ErrValidation |
| Answer | нет | string | Ответ как строка | "" |
| Year | нет | int | Год | 0 |
| ExamType | нет | string | "ege" / "oge" | "" |
| TaskType | нет | string | Подтип задания | "" |
| Solution | нет | string | Решение | "" |
| Metadata | нет | map | Произвольный JSON | nil (станет {}) |

отдельно стоит уточнить что для Metadata имеются на данный момент следующие ключи:

| Ключ            | Тип            | Описание                                  |
|-----------------|----------------|-------------------------------------------|
| `difficulty`    | string         | `"base"` / `"advanced"` / `"high"`, если указана. |
| `topics`        | []string       | Темы/подтемы.                            |
| `options`       | []string       | Варианты ответа для заданий с выбором.   |
| `images`        | []string       | URL картинок.                            |
| `formula`       | string         | LaTeX или исходная разметка формул.      |
| `variant_number`| int            | Номер варианта, если задание из варианта. |

Они могут менятся. По факту БД глубоко пофиг какие именно ключи там хранятся, но хорошо бы их знать чтобы грамотно отдать бекенду. В случае если на вашем сайте есть какой-то кастомный параметр по типу сложности, но не указанный в актуальной версии контракта, прошу прописать его здесь, обновить версию, сделать коммит и PR. В идеале еще где-то уведомить команду что появился новый ключ в Metadata, чтобы бек настроил фильтр и иные алгоритмы.

### 3.2. Артефакты

| Параметр | Обязателен | Правило | При отсутствии |
| --- | --- | --- | --- |
| versionID | да | Из AddTaskResult.VersionID | — |
| typ | да | Только db.ArtifactType* | ErrValidation |
| key | нет | [a-zA-Z0-9_\-]{1,64} | "main" |
| externalURL | нет | Абсолютный http(s):// или "" | "" |
| originalFilename | нет | Имя из источника | "" |
| filePath | да (в Complete) | Относительный от DATA_ROOT | ErrValidation |
| mimeType | да (в Complete/MarkExternal) | Из sniffing содержимого | "application/octet-stream" |
| fileSize | да (в Complete) | Байты | 0 |
| checksum | да (в Complete) | "sha256:<hex>" | ErrValidation |
| errMsg | да (в Fail) | 1–500 символов | — |
### 3.3. Константы

```go
// Типы
db.ArtifactTypeImage      = "image"
db.ArtifactTypeSVG        = "svg"
db.ArtifactTypeTikZ       = "tikz"
db.ArtifactTypeAttachment = "attachment"
db.ArtifactTypePreview    = "preview"

// Статусы
db.StatusPending   = "pending"
db.StatusDone      = "done"
db.StatusFailed    = "failed"
db.StatusAbandoned = "abandoned"
db.StatusExternal  = "external"

// Хранилище
db.StorageLocal    = "local"
db.StorageExternal = "external"
```
### 3.4. Значения artifact_key

| Значение | Когда |
| --- | --- |
| "main" | Единственная картинка / единственное вложение |
| "figure_1", "figure_2" | Несколько картинок одного типа |
| "27_A", "27_B" | Файлы ЕГЭ; имя без расширения |
| "task_3_table" | Прочие вложения |
Запрещено: .., /, \, пробелы, юникод, длина > 64.

### 3.5. DATA_ROOT

- Переменная окружения. Корень хранилища в контейнере: /app/uploads. В целом в актуальной версии контракта это мое предложение как сделать, если удобнее сделать как-то иначе, предлагайте, обсудим и решим.

Путь к файлу:

```text
{DATA_ROOT}/tasks/{taskID}/v{versionNum}/{type}_{key}.{ext}
{ext} — из mimeType по таблице: image/png → png, image/jpeg → jpg, image/svg+xml → svg, text/plain → txt, application/pdf → pdf, application/vnd.ms-excel → xls, application/vnd.openxmlformats-officedocument.spreadsheetml.sheet → xlsx, иначе bin.
filePath в БД — относительный (tasks/42/v3/image_figure_1.png), без /app/uploads/.
Промежуточные файлы — в {DATA_ROOT}/tmp/, атомарный os.Rename в финальный путь.
```
## 4. Порядок вызовов

```text
AddTask → AddTaskResult
    │
    ├─ WasCreated == false → артефакты уже существуют; при status != "failed" не трогать
    │
    └─ WasCreated == true → для каждого артефакта:

        CreatePendingArtifact
            │
            ├─ скачали / отрендерили → atomic rename → CompleteArtifact
            │
            ├─ решили не качать   → MarkExternal
            │
            └─ ошибка             → FailArtifact
CompleteArtifact, MarkExternal, FailArtifact без предшествующего CreatePendingArtifact возвращают ErrNotFound.

```
## 5. Примеры

### 5.1. Задача без артефактов

```go
task := &db.Task{
    SourceURL:   "https://ege.fipi.ru/bank/task/12345",
    SourceSite:  "ege.fipi.ru",
    Subject:     "math",
    TaskNumber:  1,
    ContentText: "Найдите значение выражения...",
    Answer:      "42",
    Year:        2024,
    ExamType:    "ege",
}

res, err := database.AddTask(ctx, task)
if err != nil {
    return fmt.Errorf("add task: %w", err)
}
// res.TaskID, res.VersionID, res.VersionNum, res.WasCreated
```
### 5.2. Картинка по URL: скачивание

```go
res, err := database.AddTask(ctx, task)
if err != nil {
    return err
}
if !res.WasCreated {
    return nil
}

imageURL := "https://ege.fipi.ru/bank/img/12345_fig1.png"

// 1. pending
if err := database.CreatePendingArtifact(ctx, res.VersionID,
    db.ArtifactTypeImage, "figure_1", imageURL, "12345_fig1.png"); err != nil {
    return err
}

// 2. скачать во временный файл
tmpPath, mimeType, size, checksum, err := downloadToTemp(ctx, imageURL)
if err != nil {
    database.FailArtifact(ctx, res.VersionID, db.ArtifactTypeImage, "figure_1", err.Error())
    return err
}
defer os.Remove(tmpPath)

// 3. atomic rename
relPath := fmt.Sprintf("tasks/%d/v%d/image_figure_1.%s",
    res.TaskID, res.VersionNum, extFromMime(mimeType))
absPath := filepath.Join(dataRoot, relPath)
if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
    database.FailArtifact(ctx, res.VersionID, db.ArtifactTypeImage, "figure_1", err.Error())
    return err
}
if err := os.Rename(tmpPath, absPath); err != nil {
    database.FailArtifact(ctx, res.VersionID, db.ArtifactTypeImage, "figure_1", err.Error())
    return err
}

// 4. done
if err := database.CompleteArtifact(ctx, res.VersionID,
    db.ArtifactTypeImage, "figure_1", relPath,
    mimeType, size, checksum); err != nil {
    return err
}
```
### 5.3. Два файла ЕГЭ

```go
res, err := database.AddTask(ctx, task)
if err != nil {
    return err
}
if !res.WasCreated {
    return nil
}

files := map[string]string{
    "27_A": "https://ege.fipi.ru/bank/2024/27_A.txt",
    "27_B": "https://ege.fipi.ru/bank/2024/27_B.txt",
}

for key, url := range files {
    if err := database.CreatePendingArtifact(ctx, res.VersionID,
        db.ArtifactTypeAttachment, key, url, key+".txt"); err != nil {
        return err
    }

    tmpPath, mimeType, size, checksum, err := downloadToTemp(ctx, url)
    if err != nil {
        database.FailArtifact(ctx, res.VersionID, db.ArtifactTypeAttachment, key, err.Error())
        continue
    }
    defer os.Remove(tmpPath)

    relPath := fmt.Sprintf("tasks/%d/v%d/attachment_%s.txt",
        res.TaskID, res.VersionNum, key)
    absPath := filepath.Join(dataRoot, relPath)
    if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
        database.FailArtifact(ctx, res.VersionID, db.ArtifactTypeAttachment, key, err.Error())
        continue
    }
    if err := os.Rename(tmpPath, absPath); err != nil {
        database.FailArtifact(ctx, res.VersionID, db.ArtifactTypeAttachment, key, err.Error())
        continue
    }

    if err := database.CompleteArtifact(ctx, res.VersionID,
        db.ArtifactTypeAttachment, key, relPath,
        mimeType, size, checksum); err != nil {
        log.Printf("complete %s: %v", key, err)
    }
}
```
### 5.4. Свой SVG

```go
res, err := database.AddTask(ctx, task)
if err != nil {
    return err
}
if !res.WasCreated {
    return nil
}

if err := database.CreatePendingArtifact(ctx, res.VersionID,
    db.ArtifactTypeSVG, "main", "", ""); err != nil {
    return err
}

svgBytes, err := renderSVG(task)
if err != nil {
    database.FailArtifact(ctx, res.VersionID, db.ArtifactTypeSVG, "main", err.Error())
    return err
}

relPath := fmt.Sprintf("tasks/%d/v%d/svg_main.svg", res.TaskID, res.VersionNum)
absPath := filepath.Join(dataRoot, relPath)
if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
    database.FailArtifact(ctx, res.VersionID, db.ArtifactTypeSVG, "main", err.Error())
    return err
}
if err := os.WriteFile(absPath, svgBytes, 0o644); err != nil {
    database.FailArtifact(ctx, res.VersionID, db.ArtifactTypeSVG, "main", err.Error())
    return err
}

sum := sha256.Sum256(svgBytes)
checksum := "sha256:" + hex.EncodeToString(sum[:])

if err := database.CompleteArtifact(ctx, res.VersionID,
    db.ArtifactTypeSVG, "main", relPath,
    "image/svg+xml", int64(len(svgBytes)), checksum); err != nil {
    return err
}
```
### 5.5. Внешний ресурс без скачивания

```go
res, err := database.AddTask(ctx, task)
if err != nil {
    return err
}
if !res.WasCreated {
    return nil
}

videoURL := "https://youtube.com/watch?v=xyz"
if err := database.CreatePendingArtifact(ctx, res.VersionID,
    db.ArtifactTypeImage, "video", videoURL, ""); err != nil {
    return err
}
if err := database.MarkExternal(ctx, res.VersionID,
    db.ArtifactTypeImage, "video", videoURL, "video/mp4"); err != nil {
    return err
}
```
### 5.6. Пакетная вставка

```go
tasks := []*db.Task{
    {SourceURL: "https://x.ru/1", Subject: "math", ContentText: "..."},
    {SourceURL: "https://x.ru/2", Subject: "math", ContentText: "..."},
}

if err := database.AddTasks(ctx, tasks); err != nil {
    return fmt.Errorf("add tasks: %w", err)
}
// VersionID не возвращается; для артефактов вызвать GetTask по SourceURL
```
### 5.7. Чтение артефактов

```go
arts, err := database.GetArtifactsByVersionID(ctx, res.VersionID)
if err != nil {
    return err
}
for _, a := range arts {
    // a.Status        "pending" | "done" | "failed" | "external"
    // a.StorageType   "local" | "external"
    // a.FilePath      "" для external
    // a.ExternalURL   "" для local
    // a.ErrorMessage  непусто при failed
}
```
## 6. Ошибки

| Ошибка | Причина | Действие |
| --- | --- | --- |
| db.ErrValidation | Невалидные поля задачи или артефакта | Исправить данные, не ретраить |
| db.ErrNotFound | Строка не найдена (например, CompleteArtifact до CreatePendingArtifact) | Исправить порядок вызовов |
| Прочие | Сеть, БД | Ретраить с backoff |
**Проверка:**

```go
if errors.Is(err, db.ErrValidation) { ... }
if errors.Is(err, db.ErrNotFound)   { ... }
```
## 7. Требования к окружению

- Переменная окружения DATA_ROOT (например, /app/uploads).
- Директория {DATA_ROOT}/tmp/ доступна на запись.
- {DATA_ROOT} смонтирован как volume в docker-compose.yml и виден тому процессу, который отдаёт файлы клиенту.
**Пример:**

```yaml
services:
  parser:
    environment:
      - DATA_ROOT=/app/uploads
    volumes:
      - ./uploads:/app/uploads
```
## 8. Ограничения

- Размер одного файла: не более 50 MB. Больше — MarkExternal.
- Таймаут GET: 30 секунд.
- Разрешённые схемы externalURL: только http и https.
- checksum обязателен в CompleteArtifact: без него невозможно проверить целостность и посчитать дедупликацию.
- mimeType определяется по содержимому файла (sniffing), не по заголовку Content-Type источника.
```text

Файл: `docs/parser-contract.md`.
```
