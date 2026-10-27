# Backend: учебный каркас

Сервер на Go 1.24+. Данные учеников в /api/v1/demo/students фиктивные, хранятся в коде и доступны без входа. Реальные данные сюда передавать нельзя.

## Запуск

В PowerShell из папки backend:

```powershell
go run ./cmd/server
```

В другом терминале:

```powershell
curl.exe -i http://localhost:8080/health
curl.exe -i http://localhost:8080/api/v1/demo/students
curl.exe -i -X POST http://localhost:8080/api/v1/auth/login
```

Порт по умолчанию 8080; переменная PORT позволяет выбрать другой порт, например для Render.

## Что работает

- GET /health — состояние сервера.
- GET /api/v1/demo/students — фиктивный профиль из storage через usecase и handler.
- Каркас маршрутов авторизации, создания профиля и приглашений отвечает JSON с HTTP 501. Они не выполняют вход или регистрацию и не сохраняют данные. Не отправляйте настоящие пароли.

`/api/v1/demo/students` — только для демонстрации, без авторизации; это не API реальных профилей.

## Docker

Из папки backend при установленном и запущенном Docker:

```powershell
docker build -t edtech-backend .
docker run --rm -p 8080:8080 edtech-backend
```

Проверка с хоста: `curl.exe -i http://localhost:8080/api/v1/demo/students`.
Для проверки из контейнера понадобится отдельный клиент curl в одной Docker-сети: рабочий образ не включает оболочку и curl.

## Проверки

```powershell
go test ./...
go vet ./...
gofmt -l .
```

Прежде чем использовать аккаунты настоящих людей, необходимо реализовать хеширование паролей, сессии, авторизацию на уровне объектов, защиту приглашений, постоянную БД и тесты. Контракт и перечень незавершённых маршрутов: ../docs/contracts/auth-draft.md.
