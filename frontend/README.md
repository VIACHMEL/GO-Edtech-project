# Django — интерфейс Go backend

Django рисует HTML и обращается к Go через `backend_client.py`.
Пароли, сессии, права и предметные данные обрабатываются в Go.
Прямого подключения Django к PostgreSQL нет.

## Запуск

Из корня проекта: `./run.sh`. Скрипт запускает PostgreSQL, Go и Django.
Открыть http://127.0.0.1:8000/, остановить — Ctrl+C.

Если окружение ещё не подготовлено:

```sh
cd frontend
python3 -m venv .venv
.venv/bin/python -m pip install -r requirements.txt
```

Для отдельного запуска Django, когда Go уже работает:

```sh
cd frontend
.venv/bin/python manage.py runserver 127.0.0.1:8000 --noreload
```

`GO_API_BASE_URL` задаёт адрес Go, по умолчанию `http://127.0.0.1:8080`.
`COOKIE_SECURE=true` включает передачу cookie только через HTTPS.
Файл `.env` автоматически не читается.

## Страницы

- `/register/`: имя, email, пароль и роль teacher/student. После создания — автоматический вход.
- `/login/`, `/logout/`: вход и выход. Локальный логин `flbedo` продолжает работать.
- `/teacher/`, `/student/`: профиль из Go.
- `/teacher/students/`: настоящие ученики преподавателя.
- `/teacher/groups/`: чтение, создание, изменение и состав групп.
- `/teacher/builder/`: банк из Go; выбранный вариант пока сохраняется только в браузере.
- Календари, проверка, задания, комментарии, статистика и `/solve/`: сообщение
  «Раздел пока не подключён», с настоящим профилем и навигацией.

Демонстрационных записей в коде нет. Аккаунты и задания БД при удалении макетов
не удалялись. Парсер и SVG → TikZ этим изменением не заменяются.

## Проверки

```sh
.venv/bin/python manage.py check
.venv/bin/python manage.py test
.venv/bin/python manage.py collectstatic --noinput
.venv/bin/python -m pip check
```

Постоянных тестов Django сейчас нет. Сквозной сценарий выполняется временно,
затем сценарий и его аккаунты удаляются по решению владельца.

Подробности: [INTERACTIONS.md](INTERACTIONS.md),
[Go API](../docs/backend-explained.md).
