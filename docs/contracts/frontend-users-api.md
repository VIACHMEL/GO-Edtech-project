# Блок пользователей → Go API/Django: JSON DTO, v0.1

**Статус:** слой чтения подключён к HTTP-серверу `cmd/server`; вход и cookie описаны в [django-frontend.md](django-frontend.md). При изменениях согласовать контракт с разработчиком frontend. Источник — действующие таблицы `users`, `teacher_students`, `groups`, `group_members` (миграции 000004–000006). **Миграций для этого слоя нет.**

## Код

- `backend/internal/db/teacher_views.go`: `ListTeacherStudentViews(ctx, teacherID)` и `ListTeacherGroupViews(ctx, teacherID)` — SQL-выборки, сгруппированные по ученикам/группам; пустые группы и ученики без группы сохраняются.
- `backend/internal/teacherdata/dto.go`: DTO с `json`-тегами, не содержащие пароль или хеш токена.
- `backend/internal/teacherdata/service.go`: `NewService(store *db.DB)`, `GetProfile(ctx, userID)`, `GetTeacherStudents(ctx, teacherID, query, status)`, `GetTeacherGroups(ctx, teacherID, query, status)`.

## HTTP-маршруты

| HTTP | Сервис | Входы | Роли/доступ |
| --- | --- | --- | --- |
| `GET /api/v1/me` | `GetProfile` | `userID` — ID из проверенной сессии, **не** из query | Авторизованный пользователь, только свой профиль |
| `GET /api/v1/teacher/students?q=&status=all` | `GetTeacherStudents` | `teacherID` из сессии, `query` — поиск по имени/email/названию активной группы; `status` = `all` / `active` (пустой = `all`) | Только активный преподаватель, только свои ученики |
| `GET /api/v1/teacher/groups?q=&status=all` | `GetTeacherGroups` | `teacherID` из сессии, `query` — имя/предмет; `status` = `all` / `active` / `archived` (пустой = `all`) | Только активный преподаватель, только свои группы |

Все ID имеют тип `int64` и сериализуются JSON-числом. Время в эти DTO пока не входит. `grade` и `subject` могут быть `null`. Пустые коллекции всегда `[]`.

## Формат ответов

**Профиль:**

```json
{"id":42,"name":"Анна Иванова","email":"anna@example.invalid","role":"student","status":"active"}
```

**Ученики:**

```json
{"students":[{"id":42,"name":"Анна Иванова","email":"anna@example.invalid","grade":11,"state":"active","initials":"АИ","groups":[{"id":3,"name":"ЕГЭ информатика"}]}],"total":1,"active_count":1}
```

**Группы:**

```json
{"groups":[{"id":3,"name":"ЕГЭ информатика","subject":"Информатика","state":"active","member_count":1,"members":[{"id":42,"name":"Анна Иванова","email":"anna@example.invalid","initials":"АИ"}]}],"total":1,"active_count":1,"archived_count":0}
```

`total`, `active_count`, `archived_count` считаются **до** применения поиска и фильтра вкладки. Поэтому могут быть больше числа элементов возвращаемого массива. Список учеников содержит только активные связи/аккаунты, поэтому `status=all` и `status=active` сейчас эквивалентны. `attention` отсутствует до появления аналитики домашних работ.

Архивные группы показываются в разделе групп, но **не** прикрепляются к ученику в его списке активных групп. Состав архивной группы показывает тех участников, у кого сохраняется активная связь с преподавателем; это не исторический снимок участников.

## Ошибки и права

- `errors.Is(err, db.ErrNotFound)` — ID пользователя не существует; сервер решает, возвращать ли HTTP 404.
- `errors.Is(err, db.ErrValidation)` — пользователь не активный преподаватель, отрицательный ID или неизвестный `status`. Возможное соответствие HTTP 400/403 зависит от причины и серверной авторизации.
- Другие ошибки — PostgreSQL/контекст и внутренние сбои; не отдавать клиенту сырой SQL и DSN.
- **Критично:** `GetProfile` получает любой `userID`; сам сервис не доказывает, что он принадлежит вызывающему. Сервер обязан брать ID только из проверенной сессии и разграничивать доступ. Нельзя считать ID из URL доказательством прав.
- Данные реальных учеников нельзя выводить на публичных `/preview/*` Django. Backend проверяет доступ; Django лишь отображает разрешённый ответ.

## Что НЕ реализовано здесь

Расширенная пагинация списков учеников/групп, `attention`, оценки, прогресс, занятия, заметки, расписание, домашние работы. Старые Django mock-поля `progress`, `last_lesson`, `schedule`, `description`, `notes` и др. нельзя подменять фиктивными значениями: Django должен поддержать отсутствие этих полей до подключения остальных модулей. Взаимодействие Django с Go и формат ошибок согласуются с backend-разработчиком.

## Проверка

Вход, чтение и сохранение групп проверены HTTP-сценариями на временной PostgreSQL.
Созданные при подключении тестовые файлы удалены после выполнения по решению владельца.
Существующие тесты `internal/db` сохранены; их интеграционные проверки требуют
отдельной тестовой БД и `EDTECH_TEST_DSN`.

Фактические HTTP-маршруты и формы: [django-frontend.md](django-frontend.md).

Регистрация, профиль и приглашения через HTTP теперь реализованы: [объяснение API](../backend-explained.md).
