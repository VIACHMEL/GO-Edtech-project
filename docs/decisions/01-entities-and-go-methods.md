# GO-Edtech — сущности и необходимые Go-методы

> **Проектная модель, не реализованный API.** Единая PostgreSQL и один модульный Go-сервер. `*` — запланировано, `✓` — основа уже есть в ветке `features/task-database` (не обязательно в `main`). Методы ниже — предлагаемые названия операций сервиса/хранилища; все принимают `context.Context`, проверяют права через сервер и возвращают `error`.
>
> **Стадии:** **A** — базовый цикл MVP; **B** — доска/занятия/совместная работа; **C** — поздние функции, создавать после утверждения требований. Группы и приглашения относятся к MVP. Доска из roadmap — этап B, но её связи предусматриваем сразу.

## 1. Пользователи и доступ

| Сущность | За что отвечает | Основные методы Go | Этап |
| --- | --- | --- | --- |
| **`users`** | Единый аккаунт ученика/репетитора; данные входа и роль. Не дублируем пользователей в `students`. | `CreateUser`, `GetUser`, `UpdateUser`, `GetUserByEmail`, `DisableUser` | A |
| **`teacher_students`** | Какие преподаватели имеют доступ к каким ученикам; один ученик может иметь нескольких преподавателей. | `LinkTeacherStudent`, `UnlinkTeacherStudent`, `ListTeacherStudents`, `ListStudentTeachers` | A |
| **`groups`** | Учебная группа преподавателя. | `CreateGroup`, `GetGroup`, `UpdateGroup`, `ListTeacherGroups`, `ArchiveGroup` | A |
| **`group_members`** | Состав группы, не массив ID внутри `groups`. | `AddGroupMember`, `RemoveGroupMember`, `ListGroupMembers` | A |
| **`invitations`** | Одноразовое приглашение ученика/подключение к преподавателю или группе. | `CreateInvitation`, `PreviewInvitation`, `AcceptInvitation`, `RevokeInvitation` | A |

**Правила:** email уникален; хранятся только хеши паролей и токенов приглашений. Связи и роли проверяются сервером. Сессии можно хранить отдельным защищённым session-store (например, Redis); отдельная таблица для сессий пока не обязательна.

## 2. Общий банк заданий (уже разрабатывается)

| Сущность | За что отвечает | Основные методы Go | Этап |
| --- | --- | --- | --- |
| **`tasks` ✓** | Карточка задачи: источник, предмет, экзамен, номер, текущая версия. | `AddTask`, `AddTasks`, `GetTask`, `GetTasksByIDs`, `ListTasks` | A |
| **`task_versions` ✓** | Условия, правильный ответ и решение **конкретной версии** задачи. Вариант ссылается на версию, а не на изменяемое текущее состояние. | `GetTaskVersion`, `ListTaskVersions`, `GetPublishedTaskVersion` | A |
| **`task_artifacts` ✓** | Изображения, файлы задач, SVG, TikZ, превью, статусы обработки. **Не** фотографии решений ученика. | `CreatePendingArtifact`, `CompleteArtifact`, `FailArtifact`, `GetArtifactsByVersionID`, `ApproveArtifact`* | A |

**Правила:** импорт по внешнему ключу без дублей; опубликованные версии и использованные в работах эталонные ответы не менять. Текущий код `AddTask` и артефактов уже есть, но правила неизменяемости версий и утверждения TikZ требуют отдельной проверки/доработки.

## 3. Варианты, назначения, ответы

| Сущность | За что отвечает | Основные методы Go | Этап |
| --- | --- | --- | --- |
| **`worksheets`** | Общий шаблон варианта, созданный преподавателем. | `CreateWorksheet`, `GetWorksheet`, `ListWorksheets`, `UpdateWorksheet`, `PublishWorksheet`, `CloneWorksheet` | A |
| **`worksheet_items`** | Позиции задач внутри варианта: ссылка на **`task_versions.id`**, порядок, баллы. | `AddWorksheetItem`, `RemoveWorksheetItem`, `ReorderWorksheetItems`, `ListWorksheetItems` | A |
| **`assignments`** | **Персональный экземпляр** выданной работы: один ученик, один вариант, свой срок. Групповая выдача создаёт по записи на ученика. | `AssignWorksheet`, `AssignWorksheetToGroup`, `GetAssignment`, `ListStudentAssignments`, `ListTeacherAssignments`, `CloseAssignment` | A |
| **`attempts`** | Конкретная попытка решения: `draft → submitted → reviewed`. Закрыл браузер — открывает ту же незавершённую попытку; пересдача — новая попытка. | `StartOrResumeAttempt`, `GetAttempt`, `SubmitAttempt`, `ReopenAttempt`, `ListAssignmentAttempts`, `PublishAttemptResult` | A |
| **`attempt_answers`** | Ответ на одну позицию/подзадачу, ревизии, баллы, проверка и комментарий преподавателя. | `SaveAnswer`, `ClearAnswer`, `GetAttemptAnswers`, `AutoGradeAnswer`, `ReviewAnswer`, `GetTeacherLiveState` | A |

**Правила:** ответ сохраняется **до** сдачи. Учитель получает обновление и предварительную правильность в реальном времени (SSE/WS); ученик получает только свой ответ и статус сохранения. В API для ученика **не передавать** правильный ответ, `is_correct`, `score` или комментарий проверки до публикации результата. Контроль доступа обязателен и для HTTP, и для событий. `answer_revision` предотвращает использование оценки старой редакции. Для визуализации учителя: **−1 = не решал, 0 = неверно, 1 = верно; NULL = ответ есть, проверка ожидается**. Это вычисляемый статус, не отдельный список ID.

## 4. Файлы и интерактивная доска

| Сущность | За что отвечает | Основные методы Go | Этап |
| --- | --- | --- | --- |
| **`files`** | Загруженные пользователями фото решений, PDF и вложения к вариантам/занятиям/доскам. Сами байты лежат в файловом/object storage, а не в PostgreSQL. Артефакты из парсеров остаются в `task_artifacts`. | `RegisterFile`, `GetFile`, `ListFilesForTarget`, `DeleteFile`, `GetAuthorizedDownload` | A/B |
| **`boards`** | Сама доска: владелец, название, JSON-состояние рисунка, номер ревизии. | `CreateBoard`, `GetBoard`, `RenameBoard`, `SaveBoardSnapshot`, `DeleteBoard` | B |
| **`board_links`** | **К чему прикреплена доска:** версии задачи, варианту, задаче варианта, личному назначению, личной задаче назначения или занятию. Одна доска может иметь несколько привязок. | `AttachBoard`, `DetachBoard`, `ListBoardsForTarget`, `SetBoardLinkAccess` | B |
| **`board_operations`** | Изменения совместной доски: штрихи, стирание, перенос объектов. Нужны для упорядочивания, синхронизации и восстановления после обрыва соединения. | `ApplyBoardOperations`, `GetBoardOperationsSince`, `CompactBoardOperations` | B |

**Правила доски:** `board_links` разрешает только допустимые комбинации внешних ключей, а для личной задачи — пару `(assignment_id, worksheet_item_id)` из **того же варианта**. Преподавательская доска-шаблон не становится автоматически доступной всем ученикам: режим доступа задаётся явно. Совместное редактирование передаётся через WebSocket; снимок JSON + упорядоченный журнал изменений восстанавливают доску. Картинки/PDF в доске — ссылки на `files`/`task_artifacts`, не base64 внутри JSON.

## 5. Занятия и перспективные модули

| Сущность | За что отвечает | Основные методы Go | Этап |
| --- | --- | --- | --- |
| **`lessons`** | Расписание, преподаватель, время, статус, ссылка на видеокомнату. Доска привязывается через `board_links`. | `CreateLesson`, `UpdateLesson`, `CancelLesson`, `StartLesson`, `FinishLesson`, `ListLessons` | B |
| **`lesson_participants`** | Участники конкретного занятия (не обязательно постоянная группа). | `AddLessonParticipant`, `RemoveLessonParticipant`, `ListLessonParticipants`, `MarkAttendance` | B |
| **`notifications`** | Уведомления о назначении, сроке, проверке, занятии. | `CreateNotification`, `ListNotifications`, `MarkNotificationRead` | C |
| **`subscriptions`** | Подписка репетитора на тариф и срок действия доступа. | `CreateSubscription`, `GetActiveSubscription`, `ChangePlan`, `CancelSubscription` | C |
| **`payments`** | История платежей/возвратов и статусов платёжного провайдера. | `RecordPayment`, `GetPayment`, `ListPayments`, `ApplyPaymentWebhook` | C |
| **`ai_jobs`** | Асинхронные задания ИИ: проверка, рекомендации, конвертация/анализ; результат, модель, статус. | `EnqueueAIJob`, `ClaimAIJob`, `CompleteAIJob`, `FailAIJob`, `GetAIJob` | C |

**Не создаём отдельные таблицы** для «решённых ID», сводной статистики, состояния «ученик онлайн», видеопотока, каждой фигуры доски и каждой подсказки ИИ: они вычисляются из базовых данных, хранятся в операционном состоянии или в `JSONB`/файлах. Аналитика сначала считается по `attempt_answers`; таблицы агрегатов добавляются лишь при доказанной необходимости.

## 6. Как разделить разработку

| Пакет | Кому поручить | Входит | Зависимости |
| --- | --- | --- | --- |
| **P1: Аккаунты** | Разработчик сервера/авторизации | `users`, `teacher_students`, `groups`, `group_members`, `invitations` | Нет |
| **P2: Банк** | Разработчик БД + парсеров | `tasks`, `task_versions`, `task_artifacts` | Уже в работе |
| **P3: Варианты** | Разработчик работ | `worksheets`, `worksheet_items`, `assignments` | Контракты `users.id`, `task_versions.id` |
| **P4: Решение** | Разработчик исполнения | `attempts`, `attempt_answers`, проверка и SSE | Контракт P3 |
| **P5: Доска** | Разработчик доски | `files`, `boards`, `board_links`, `board_operations` | Контракты P2/P3; можно начать автономно |
| **P6: Занятия** | Разработчик занятий | `lessons`, `lesson_participants`, интеграция с доской/видео | Контракт P1/P5 |
| **Позже** | По результатам MVP | `notifications`, `subscriptions`, `payments`, `ai_jobs` | Утверждённые бизнес-сценарии |

**Общее для каждой задачи:** миграция `up/down`, Go-модели и методы, проверка доступа на сервере, тесты с PostgreSQL, обработка повторных запросов, согласованные HTTP-контракты. Не отдавать `DB.Pool` напрямую в frontend. Изменения в один и тот же SQL-файл/миграцию между участниками не делать.

**Фиксируем до начала кодирования:** ID = `BIGINT/int64`; формат файлового хранилища; кто владелец доски и может её менять; что означает повторная выдача/пересдача; способ публикации оценок. Даты/счётчики, журнал доски и авто-проверка должны выдерживать повтор запроса и обрыв соединения.
