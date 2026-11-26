# Развёртывание EdTech на VPS — 26.11.2026

> Историческая инструкция первого выпуска, миграций и импорта. Для обычных обновлений после успешного выпуска PR #25 использовать [быстрый регламент](quick-vps-release.md) и актуальный `bash deploy.sh`. Не повторять уже применённые миграции и не импортировать банк при редизайне. PR #25 включил PR #24; отдельное слияние #24 не требуется.

## 1. Версия и назначение

Тестовый стенд: https://edtech.promodetector.top/.
Целевая версия приложения: `main`, коммит `2c8db3d7a69a06740727ec5e773503136cafc675` (merge PR #22). Исходники были переданы через `git archive origin/main` и SCP, а не через `git clone` на VPS. Точный SHA загруженного архива ещё необходимо сверить командой ниже. Этот документ фиксирует первоначальный запуск по журналам терминала, а не результат полной приёмки.

Не включать в Git пароли SSH, токены сессий, DATABASE_URL, секрет Django, содержимое `/etc/edtech-stage/app.env`, персональные данные и дампы БД.

## 2. Инфраструктура

| Компонент | Конфигурация |
| --- | --- |
| VPS | Ubuntu 24.04.4 LTS, x86_64, 2 vCPU, 1,9 ГиБ RAM, диск 58 ГБ |
| Swap | 2 ГиБ, `/swap-edtech`, подключение через `/etc/fstab` |
| Go | 1.26.11, `/opt/edtech-toolchain/go/bin/go` |
| Node.js / npm | 22.23.5 / 10.9.9, `/opt/edtech-toolchain/node/bin` |
| Python / Django / Gunicorn | 3.12.3 / 5.2.18 / 27.0.1 |
| PostgreSQL | 16.15, существующий кластер `16/main`, loopback:5432 |
| БД и роль приложения | `edtech_stage_db`, `edtech_stage_app` |
| Системный пользователь | `edtechstage` (без интерактивного входа) |
| Go | `127.0.0.1:18080`, `edtech-stage-go.service` |
| Django | `127.0.0.1:18081`, `edtech-stage-django.service` |
| Reverse proxy / TLS | Существующий Caddy, 80/443, сертификат Let's Encrypt |
| DNS | Отдельная A-запись `edtech` у существующего домена PromoRadar, DNS only |

Другие приложения на VPS сохранены. В частности, Caddy обслуживает также `ludomania.shop`, а порт 8000 занят другим Python-приложением. Не менять существующие DNS-записи, туннель PromoRadar, чужие БД и службы. Общий VPS означает общие ресурсы и не является полной изоляцией безопасности.

Поток запросов: браузер HTTPS → Caddy → Go; Go обслуживает API/WebSocket и проксирует страницы в Django. Статические файлы Caddy отдаёт напрямую. Django обращается к Go по loopback. PostgreSQL недоступен напрямую извне по наблюдавшимся listening sockets.

## 3. Что сделано

- Обновлён `main` через PR #22; в нём Go HTTP API, Django, интерактивная доска и сохранение черновиков.
- Загружен `/root/edtech-main.tar.gz`, распакован отдельный стенд `/opt/go-edtech-stage`; прежний `/opt/go-edtech` не заменяли.
- Созданы отдельный Linux-пользователь, роль PostgreSQL и база приложения; пароль БД генерировался локально, конфигурация подключения восстановлена после ошибки вставки Bash.
- Применены миграции 000001–000011 одной транзакцией; в схеме 16 таблиц.
- Собран Go-бинарник (16 МБ), создан Python venv, установлен Gunicorn; `manage.py check` не выявил ошибок.
- Выполнены `npm ci`, `npm run build` для Excalidraw (с предупреждениями peer dependencies React/Radix; runtime не проверен), `collectstatic` (113 файлов).
- Созданы службы systemd с автозапуском от `edtechstage`, добавлены локальные настройки Django для HTTPS.
- Добавлен отдельный блок Caddy, проверена конфигурация, выполнен reload и получен TLS-сертификат.
- Добавлены 3 собственных демонстрационных задачи ОГЭ по математике (№1, 2, 20) с версиями и метаданными части экзамена. Это не импорт настоящего банка.
- Создан первый ручной дамп `/var/backups/edtech-stage/db-20261125-220934.dump` (131 КБ); `pg_restore -l` прочитал каталог архива. Полное восстановление и внешнее резервирование не проверены.

## 4. Файлы и настройки стенда

| Путь | Назначение |
| --- | --- |
| `/opt/go-edtech-stage/bin/edtech-server` | Go-бинарник |
| `/opt/go-edtech-stage/frontend/.venv` | Окружение Django/Gunicorn |
| `/opt/go-edtech-stage/frontend/config/edtech_vps_settings.py` | Локальные настройки Django: постоянный SECRET_KEY из окружения, DEBUG=False, SECURE_PROXY_SSL_HEADER, CSRF_TRUSTED_ORIGINS, secure cookies |
| `/opt/go-edtech-stage/frontend/staticfiles` | Собранная статика, доступная Caddy |
| `/etc/edtech-stage/app.env` | Секреты и переменные окружения, root-only |
| `/etc/systemd/system/edtech-stage-go.service` | Go, работа от edtechstage |
| `/etc/systemd/system/edtech-stage-django.service` | Gunicorn, 1 worker, 4 threads |
| `/var/lib/edtech-stage/uploads` | Приватные фотографии ответов |
| `/etc/caddy/Caddyfile` | Общая конфигурация Caddy |
| `/var/backups/edtech-stage` | Ручные резервные копии |

Переменные окружения без секретных значений:

```dotenv
BACKEND_ADDR=127.0.0.1:18080
GO_API_BASE_URL=http://127.0.0.1:18080
DJANGO_PROXY_URL=http://127.0.0.1:18081
BOARD_ORIGIN=https://edtech.promodetector.top
DJANGO_ALLOWED_HOSTS=edtech.promodetector.top,127.0.0.1,localhost
DJANGO_DEBUG=false
DJANGO_SETTINGS_MODULE=config.edtech_vps_settings
COOKIE_SECURE=true
UPLOAD_DIR=/var/lib/edtech-stage/uploads
```

Также требуются `DATABASE_URL` и `DJANGO_SECRET_KEY`, значения хранятся только на сервере.

Добавленный блок Caddy:

```caddyfile
edtech.promodetector.top {
    encode gzip zstd
    handle_path /static/* {
        root * /opt/go-edtech-stage/frontend/staticfiles
        file_server
    }
    handle {
        reverse_proxy 127.0.0.1:18080
    }
}
```

## 5. Что подтверждено проверками

- Оба сервиса EdTech, PostgreSQL, Caddy и mai-ludomania были `active`; оба EdTech-сервиса `enabled`.
- Go `/health` возвращал `{"status":"ok"}` локально и по HTTPS.
- `/login/` локально через Go с корректным Host возвращал HTTP 200.
- Caddy получил сертификат для поддомена; JS-файл доски возвращал HTTP 200.
- HEAD `/` возвращал 405 с `Allow: GET`: главная страница ограничена GET. Проверять обычным GET, а не `curl -I`.
- Банк содержит три задания и ненулевые `current_version_id`.
- После запуска без нагрузки: 538 МиБ used, 1,4 ГиБ available, 59 МиБ swap, 40 ГБ свободного диска. Нагрузочных испытаний не проводили.
- Пользователь подтвердил отображение заглушек календаря, статистики и комментариев в браузере. Полный пользовательский сценарий пока не подтверждён.

## 6. Почему пропали разделы старой локальной версии

Старая версия для локального просмотра: `cc97211c7ae1c38df179ac697a581107c4d253f2`.
В коммите [`7f2f45a`](https://github.com/VIACHMEL/GO-Edtech-project/commit/7f2f45a20402ba30f1e30a6286bcd1ee10993896) удалены демонстрационные данные из `frontend/backend_client.py`, соответствующие обработчики и шаблоны. Маршруты переведены на `not_connected`.
Коммит [`f219249`](https://github.com/VIACHMEL/GO-Edtech-project/commit/f219249672ab0ff119723a2866204fb4198e3dbb) подключил реальные назначения и решение работ с доской; остальные разделы оставлены отключёнными.
Коммит [`4b842d8`](https://github.com/VIACHMEL/GO-Edtech-project/commit/4b842d861a0f8559dafa46873f4feb91dca2b286) добавил сохранение черновиков и защиту несохранённых ответов.

| Раздел | Состояние в коде текущего main |
| --- | --- |
| Регистрация, вход, профиль | Реальные запросы к Go API |
| Ученики, группы | Реальные данные и операции; интерфейс приглашений отключён |
| Банк | Реальные данные из БД, на стенде пока 3 тестовых задачи |
| Конструктор | Черновики в БД, ревизии, публикация и назначение |
| Назначенные задания и решение | Реальные назначения/попытки, сохранение ответов и сдача |
| Доска | Интеграция с попытками, WebSocket, хранение в PostgreSQL |
| Календарь преподавателя и ученика | `not_connected`; старые страницы работали на demo-данных |
| Статистика и комментарии ученика | `not_connected`; старые страницы работали на demo-данных |
| Проверка работ преподавателем | `not_connected`; Go API проверки существует, интерфейс отсутствует |

Удалённые шаблоны: `calendar_content.html`, `teacher_calendar.html`, `student_calendar.html`, `student_statistics.html`, `student_comments.html`, `teacher_reviews.html`, `student_assignments.html`. Последний заменён реальным интерфейсом `live_assignments.html`.

Отсутствие этих разделов объясняется изменением исходного кода, а не Caddy или PostgreSQL. Это визуальная регрессия относительно демонстрации. Не откатывать весь main и не возвращать вымышленные данные как реальные. Согласовать с командой восстановление дизайна старых шаблонов и подключение настоящих данных; если функций пока нет, показывать пустое состояние или явную отметку «Демо».

## 7. Управление и проверка

На VPS:

```bash
systemctl status edtech-stage-go edtech-stage-django --no-pager
journalctl -u edtech-stage-go -u edtech-stage-django -n 100 --no-pager
curl -fsS https://edtech.promodetector.top/health
curl -sS -o /dev/null -w 'HTTP %{http_code}\n' https://edtech.promodetector.top/
free -h
df -h /
```

Проверка SHA первоначального архива и неизменности файла маршрутизации:

```bash
gzip -dc /root/edtech-main.tar.gz | git get-tar-commit-id
tar -xOf /root/edtech-main.tar.gz frontend/config/urls.py | sha256sum
sha256sum /opt/go-edtech-stage/frontend/config/urls.py
```

Сверить SHA с коммитом приложения и два хеша между собой. Архив не является Git-репозиторием: `git pull` в директории стенда не предусмотрен.

Перезапуск только приложения (сеансы Go хранятся в памяти, после перезапуска потребуется повторный вход):

```bash
systemctl restart edtech-stage-django
systemctl restart edtech-stage-go
```

## 8. Последующие обновления и откат

Текущие `run.sh` и `deploy.sh` из Git рассчитаны на другую схему (порты 8080/8000, `/opt/go-edtech`, Django runserver). Не запускать их на этом стенде без адаптации.

Порядок нового релиза: проверить целевой SHA и тесты, создать архив из проверенного main, сохранить текущую версию кода и конфигурации, создать резервные копии БД и uploads, подготовить сборку в отдельной директории, согласовать миграции и короткое окно обслуживания, переключить только службы EdTech, проверить GET/health/статику и пользовательские сценарии. Не копировать локальную `.env`, `.venv`, данные PostgreSQL и не повторять уже применённые миграции. Миграции в новой версии проверять отдельно; откат кода не гарантирует совместимость с новой схемой. Отдельный автоматизированный скрипт обновления и rollback пока не созданы.

### Релиз банка ЕГЭ по информатике

Порядок выпуска: отправить `update_kege_parser` на GitHub → принять PR №24 в
`main` → открыть и слить PR с банком ЕГЭ → собрать архив из SHA слияния →
подготовить сборку на VPS → сохранить резервную копию → применить миграции →
импортировать 270 задач → переключить службы → проверить сайт по HTTPS.

#### Что вошло в релиз

| Изменение | Причина и реализация |
| --- | --- |
| Отбор и импорт КЕГЭ | ID не определяет номер задания. Импортёр читает номер из ответа КЕГЭ, отсеивает дубликаты, проверяет материалы и набирает по 10 задач на номер. Для №20 и №21 поддерживает ссылки на подзадачи вида `31516:20`. Файлы хранятся под `DATA_ROOT`, в БД записываются относительный путь, размер и SHA-256. |
| Условия и материалы | Обычного текста не хватает для картинок и таблиц. Миграция `000013_task_conditions` добавляет разметку к версии задачи и заполняет её из метаданных текущих версий КЕГЭ. Go оставляет разрешённые теги и подставляет защищённые ссылки. Каталог и решалка показывают условия без выдачи эталонных ответов и решений. |
| Сохранение ответа | Старая вкладка могла прочитать CSRF из HTML до повторного входа. Решалка берёт актуальный токен перед сохранением и сохраняет введённый текст в поле при отказе сервера или конфликте ревизий. |
| Доска | PR №24 добавил ревизии объектов, восстановление черновика и досылку операции с тем же ID, загрузку PNG/JPEG и экспорт. Его миграция `000012_board_sync` меняет схему доски, API и собранная статика обновляются вместе. Импортёр размечает №1–23 как общую доску и №24–27 как отдельные. Существующие снимки с явно заданной частью сохраняются. |

Локально проверены 270 заданий, 158 файлов, пять примеров в пользовательском варианте и полный сценарий из 27 позиций в браузере. Проверены сохранение после обновления CSRF, конфликт ревизий, обрыв и восстановление доски, сдача. Миграцию проверили на заполненной БД и с нуля. Результаты относятся к локальной среде.

#### GitHub · PowerShell на рабочем компьютере

Ветка `update_kege_parser` уже содержит проверенный коммит `245c35f` из [PR №24](https://github.com/VIACHMEL/GO-Edtech-project/pull/24). Её можно отправить на GitHub, пока №24 в Draft. Открывайте PR с банком ЕГЭ после принятия №24 в `main`, чтобы diff содержал только изменения банка и интеграции.

Из корня репозитория проверьте ветку и сравните её с актуальным `main`. Если в `origin/main` появились изменения в миграциях или тех же модулях, сначала разберите пересечения в PR.

```powershell
if ((git branch --show-current) -ne 'update_kege_parser') { throw 'Ожидалась ветка update_kege_parser' }
git status --short
git fetch origin
if ($LASTEXITCODE -ne 0) { throw 'Не удалось обновить origin' }
git log --oneline --left-right HEAD...origin/main
git diff --check
$releasePaths = @(
    'backend/README.md', 'backend/cmd/kompege',
    'backend/cmd/server/main.go', 'backend/cmd/server/teacher.go',
    'backend/internal/boardhttp', 'backend/internal/db/boards.go',
    'backend/internal/db/tasks.go', 'backend/internal/db/task_conditions.go',
    'backend/internal/db/migrations/000013_task_conditions.up.sql',
    'backend/internal/db/migrations/000013_task_conditions.down.sql',
    'docs/contracts/django-frontend.md', 'docs/deployment-vps.md',
    'frontend/solve-board/README.md', 'frontend/static/solve.js',
    'frontend/static/styles.css', 'frontend/templates/solve.html',
    'frontend/templates/teacher_builder.html'
)
git add -- @releasePaths
git diff --cached --check
git diff --cached --stat
git diff --cached --name-only
```

Проверьте список файлов перед коммитом. `.env`, `selected.tsv`, файлы заданий, дампы, окружение Python, `node_modules` и собранная статика передаются или собираются отдельно от Git.

```powershell
git commit -m "feat: добавить банк ЕГЭ и показ материалов заданий"
if ($LASTEXITCODE -ne 0) { throw 'Коммит не создан' }
git push -u origin update_kege_parser
if ($LASTEXITCODE -ne 0) { throw 'Ветка не отправлена' }
```

После ревью снимите Draft у PR №24 и выберите **Create a merge commit**. Перед слиянием сверьте последний коммит №24 с `245c35f`. Если ветка доски получила новые правки, сначала подтяните их и повторите локальные проверки. После squash или rebase потребуется отдельно проверить историю и конфликты.

```powershell
git fetch origin
if ($LASTEXITCODE -ne 0) { throw 'Не удалось обновить origin' }
git merge-base --is-ancestor 245c35f origin/main
if ($LASTEXITCODE -ne 0) { throw 'Проверенный коммит PR №24 ещё не включён в origin/main' }
git diff --stat origin/main...HEAD
gh pr create --base main --head update_kege_parser --fill
```

В PR проверьте `base: main` и `compare: update_kege_parser`. После ревью и обязательных проверок нажмите **Merge pull request** на GitHub и скопируйте полный SHA результата слияния. Для создания PR без CLI можно открыть страницу сравнения. [Сравнить ветки](https://github.com/VIACHMEL/GO-Edtech-project/compare/main...update_kege_parser) · [Создание PR через gh](https://cli.github.com/manual/gh_pr_create) · [Слияние PR](https://docs.github.com/en/pull-requests/how-tos/merge-and-close-pull-requests/merging-a-pull-request)

После слияния соберите архив именно из этого SHA. Подставьте свой SSH-адрес. Передайте код и список ID, а материалы скачает импортёр на VPS.

```powershell
git fetch origin
if ($LASTEXITCODE -ne 0) { throw 'Не удалось обновить origin' }
$sha = 'ПОЛНЫЙ_SHA_ИЗ_СЛИТОГО_PR'
if ($sha -notmatch '^[0-9a-f]{40}$') { throw 'Укажите полный SHA' }
git merge-base --is-ancestor $sha origin/main
if ($LASTEXITCODE -ne 0) { throw 'SHA отсутствует в истории origin/main' }
foreach ($migration in @('000012_board_sync.up.sql', '000013_task_conditions.up.sql')) {
    git cat-file -e "${sha}:backend/internal/db/migrations/$migration"
    if ($LASTEXITCODE -ne 0) { throw "В SHA нет нужной миграции $migration" }
}
$archive = "edtech-$sha.tar.gz"
$manifest = 'data/raw/kompege-selection-20261126-100849/selected.tsv'
$vps = 'root@АДРЕС_VPS'
git archive --format=tar.gz --output=$archive $sha
if ($LASTEXITCODE -ne 0) { throw 'Архив не создан' }
scp $archive "${vps}:/root/$archive"
if ($LASTEXITCODE -ne 0) { throw 'Архив не передан' }
scp $manifest "${vps}:/root/edtech-$sha-selected.tsv"
if ($LASTEXITCODE -ne 0) { throw 'Список ID не передан' }
```

#### Подготовка и выкладка · Bash на VPS

Начните с проверки схемы стенда по разделам 2, 4 и 7. Блоки ниже рассчитаны на текущие пути `/opt/go-edtech-stage`, БД `edtech_stage_db` и службы `edtech-stage-*`. Выполняйте их в одной root-сессии Bash. Остановитесь при расхождениях с инвентаризацией и запланируйте окно обслуживания. Перезапуск Go завершит текущие сеансы.

```bash
set -euo pipefail
sha='ПОЛНЫЙ_SHA_ИЗ_СЛИТОГО_PR'
[[ "$sha" =~ ^[0-9a-f]{40}$ ]]
app=/opt/go-edtech-stage
release="$app/releases/$sha"
db=edtech_stage_db
envfile=/etc/edtech-stage/app.env
data=/var/lib/edtech-stage/task-data
manifest="/root/edtech-$sha-selected.tsv"
archive="/root/edtech-$sha.tar.gz"
export PATH="/opt/edtech-toolchain/go/bin:/opt/edtech-toolchain/node/bin:$PATH"
systemctl show -p User -p WorkingDirectory -p ExecStart -p EnvironmentFiles \
  edtech-stage-go edtech-stage-django
df -h "$app" /var/lib/edtech-stage
[[ "$(gzip -dc "$archive" | git get-tar-commit-id)" = "$sha" ]]
test -f "$envfile"
test -f "$app/frontend/config/edtech_vps_settings.py"
test -f "$manifest"
systemd-run --quiet --wait --pipe --collect --uid=edtechstage \
  -p "EnvironmentFile=$envfile" /usr/bin/python3 -c '
import os
import sys
from urllib.parse import unquote, urlsplit
dsn = urlsplit(os.environ["DATABASE_URL"])
if dsn.scheme not in {"postgres", "postgresql"} or dsn.hostname not in {"localhost", "127.0.0.1", "::1"} or unquote(dsn.path) != "/" + sys.argv[1]:
    sys.exit("DATABASE_URL does not point to the expected local EdTech database")
' "$db"
```

Подготовьте сборку до остановки служб. Версия Gunicorn соответствует описанному стенду. Сборка Django берёт локальные настройки HTTPS из действующего стенда.

```bash
test ! -e "$release"
install -d -m 0755 "$release/bin"
tar --no-same-owner -xzf "$archive" -C "$release"
install -m 0644 "$app/frontend/config/edtech_vps_settings.py" \
  "$release/frontend/config/edtech_vps_settings.py"
chown -R edtechstage:edtechstage "$release"
runuser -u edtechstage -- env PATH="$PATH" bash -ec '
  cd "$1/backend"
  go build -o "$1/bin/edtech-server" ./cmd/server
  go build -o "$1/bin/kompege" ./cmd/kompege
  cd "$1/frontend/solve-board"
  npm ci --no-audit --no-fund
  npm run build
  cd ..
  python3 -m venv .venv
  .venv/bin/python -m pip install -r requirements.txt gunicorn==27.0.1
  .venv/bin/python -m pip check
' bash "$release"
systemd-run --quiet --wait --pipe --collect --uid=edtechstage \
  --working-directory="$release/frontend" -p "EnvironmentFile=$envfile" \
  "$release/frontend/.venv/bin/python" manage.py check
systemd-run --quiet --wait --pipe --collect --uid=edtechstage \
  --working-directory="$release/frontend" -p "EnvironmentFile=$envfile" \
  "$release/frontend/.venv/bin/python" manage.py collectstatic --noinput
test -s "$release/frontend/staticfiles/solve-board/dist/solve-board.js"
caddy_user="$(systemctl show -p User --value caddy)"
runuser -u "${caddy_user:-root}" -- test -r \
  "$release/frontend/staticfiles/solve-board/dist/solve-board.js"
```

Проверьте переданный список. Для первого выпуска `DATA_ROOT` задан отдельным файлом настроек службы Go. Если он уже задан в `app.env` или другом override, согласуйте один общий путь для службы и импортёра, прежде чем продолжать.

```bash
awk -F '\t' '
  { sub(/\r$/, "") }
  NR==1 { if ($1!="number" || $2!="source_ref" || $3!="materials") exit 1; next }
  NF!=3 || $1 !~ /^[0-9]+$/ || $1<1 || $1>27 ||
    $2 !~ /^[0-9]+(:(20|21))?$/ || $3 !~ /^[0-9]+$/ || seen[$2]++ { exit 1 }
  { count[$1]++; files+=$3 }
  END { if (NR!=271 || files!=158) exit 1
        for (i=1;i<=27;i++) if (count[i]!=10) exit 1 }
' "$manifest"
mapfile -t ids < <(tail -n +2 "$manifest" | cut -f2)
test "${#ids[@]}" -eq 270
override=/etc/systemd/system/edtech-stage-go.service.d/20-task-data.conf
test ! -e "$override"
test ! -e "$app/frontend.before-$sha"
test ! -e "$app/bin/edtech-server.before-$sha"
```

Следующий блок поддерживает стенд после `000011` или после `000012_board_sync`. Если стенд ещё до `000012_board_sync`, заранее согласуйте восстановление снимка БД в окно обслуживания. Миграция удаляет прежний столбец операций доски. После её применения бинарник до PR №24 несовместим со схемой. Блок сначала проверяет схему, затем останавливает только службы EdTech и создаёт резервную копию.

```bash
schema="$(runuser -u postgres -- psql -XAt -d "$db" -v ON_ERROR_STOP=1 -c "
SELECT EXISTS(SELECT FROM information_schema.columns
 WHERE table_schema='public' AND table_name='worksheets' AND column_name='revision'),
       EXISTS(SELECT FROM information_schema.columns
 WHERE table_schema='public' AND table_name='task_versions' AND column_name='condition_html'),
       to_regclass('public.board_files') IS NOT NULL,
       EXISTS(SELECT FROM information_schema.columns
 WHERE table_schema='public' AND table_name='board_operations' AND column_name='payload_digest');")"
migrations=()
case "$schema" in
  't|f|f|f') migrations+=(-f "$release/backend/internal/db/migrations/000012_board_sync.up.sql") ;;
  't|f|t|t') ;;
  *) printf 'Неожиданная схема БД: %s\n' "$schema" >&2; exit 1 ;;
esac
systemctl stop edtech-stage-go edtech-stage-django
umask 077
backup="$(mktemp -d "/var/backups/edtech-stage/release-${sha:0:12}-XXXXXXXX")"
printf 'Резервная копия: %s\n' "$backup"
runuser -u postgres -- pg_dump -Fc -d "$db" > "$backup/db.dump"
pg_restore -l "$backup/db.dump" > "$backup/db.list"
test -s "$backup/db.list"
cp -a "$envfile" "$backup/app.env"
storage=(uploads)
if [[ -d "$data" ]]; then storage+=(task-data); fi
tar -cpf "$backup/files.tar" -C /var/lib/edtech-stage "${storage[@]}"
tar -tf "$backup/files.tar" > "$backup/files.list"
test -s "$backup/files.list"
printf '%s\n' "$schema" > "$backup/schema-before.txt"
migrations+=(-f "$release/backend/internal/db/migrations/000013_task_conditions.up.sql")
runuser -u postgres -- psql -X -d "$db" -v ON_ERROR_STOP=1 --single-transaction \
  "${migrations[@]}"
if [[ ! -d "$data" ]]; then install -d -o edtechstage -g edtechstage -m 0750 "$data"; fi
```

#### Импорт 270 заданий и проверка

Запустите импорт из той же root-сессии. В случае разрыва SSH восстановите переменные
из первого блока Bash и укажите напечатанный путь `backup`. Пока служба
`edtech-kompege-<12 символов SHA>` активна, дождитесь завершения. Если прежний
журнал уже содержит 270 сохранений, сразу переходите к сверке. При повторе
команда снова читает ID из TSV. Миграции пропускаются.

```bash
test -s "$backup/db.dump"
mapfile -t ids < <(tail -n +2 "$manifest" | cut -f2)
test "${#ids[@]}" -eq 270
log="$(mktemp "$backup/kompege-XXXXXXXX.log")"
printf 'Журнал импорта: %s\n' "$log"
systemd-run --quiet --wait --pipe --collect --uid=edtechstage \
  --unit="edtech-kompege-${sha:0:12}" \
  -p "EnvironmentFile=$envfile" --setenv="DATA_ROOT=$data" \
  "$release/bin/kompege" "${ids[@]}" > "$log" 2>&1
test "$(grep -c '^Сохранено:' "$log")" -eq 270
```

Импорт идёт последовательно и может занять несколько минут. При остановке на ошибке проверьте журнал и причину. Успешные записи остаются в PostgreSQL. Перед возобновлением сверьте уже записанные ID и сохранённые файлы. Следующая проверка сравнивает все строки TSV с текущими версиями и проверяет файлы по SHA-256.

```bash
{
  printf 'CREATE TEMP TABLE expected(number int, source_ref text, materials int);\n'
  printf '%s\n' "COPY expected FROM STDIN WITH (FORMAT csv, DELIMITER E'\\t', HEADER true);"
  awk '{ sub(/\r$/, ""); print }' "$manifest"
  printf '\\.\n'
  cat <<'SQL'
DO $check$
BEGIN
  IF EXISTS (
    SELECT FROM expected e
    LEFT JOIN tasks t ON t.source_url='https://kompege.ru/api/v1/task/'||replace(e.source_ref,':','#')
      AND t.exam_type='ege' AND t.subject='informatics' AND t.task_number=e.number
    LEFT JOIN task_versions v ON v.id=t.current_version_id AND v.task_id=t.id
    WHERE v.id IS NULL OR btrim(v.content_text)='' OR btrim(v.answer)='' OR v.condition_html=''
       OR (SELECT count(*) FROM task_artifacts a WHERE a.task_version_id=v.id
             AND a.status='done' AND a.storage_type='local')<>e.materials
  ) THEN RAISE EXCEPTION 'Квота, версии или материалы расходятся с TSV'; END IF;
END
$check$;
SELECT replace(a.checksum,'sha256:','')||'  '||a.file_path
FROM expected e JOIN tasks t
  ON t.source_url='https://kompege.ru/api/v1/task/'||replace(e.source_ref,':','#')
JOIN task_artifacts a ON a.task_version_id=t.current_version_id
WHERE a.status='done' AND a.storage_type='local';
SQL
} | runuser -u postgres -- psql -XqAt -d "$db" -v ON_ERROR_STOP=1 > "$backup/materials.sha256"
test "$(wc -l < "$backup/materials.sha256")" -eq 158
awk '$1 !~ /^[0-9a-f]+$/ || length($1)!=64 ||
       $2 !~ /^kompege\/[0-9]+\/[0-9a-f]+\.[a-z0-9]+$/ || NF!=2 {exit 1}' \
  "$backup/materials.sha256"
( cd "$data"; sha256sum --status -c "$backup/materials.sha256" )
```

#### Переключение служб и приёмка

Переключите стабильные пути служб и Caddy на подготовленную сборку. Старые каталоги и бинарник сохраняются рядом для отката. Python-окружение остаётся по абсолютному пути, где его создали. После `umask 077` задайте права каталога override явно.

```bash
install -d -m 0755 "$(dirname "$override")"
printf '[Service]\nEnvironment=DATA_ROOT=%s\n' "$data" > "$override"
mv -T "$app/frontend" "$app/frontend.before-$sha"
ln -sT "$release/frontend" "$app/frontend"
mv -T "$app/bin/edtech-server" "$app/bin/edtech-server.before-$sha"
ln -sT "$release/bin/edtech-server" "$app/bin/edtech-server"
systemctl daemon-reload
systemctl start edtech-stage-django edtech-stage-go
systemctl is-active edtech-stage-django edtech-stage-go
curl -fsS https://edtech.promodetector.top/health
curl -fsS -o /dev/null https://edtech.promodetector.top/login/
curl -fsS -o /dev/null https://edtech.promodetector.top/static/solve-board/dist/solve-board.js
```

На HTTPS войдите отдельным тестовым преподавателем. В каталоге проверьте №1 с картинкой, №2 с таблицей, №3 с картинками и файлом. Назначьте вариант тестовому ученику. Проверьте скачивание, сохранение ответа, доску через `wss://` и повторное открытие после обновления страницы. Локальные тестовые аккаунты и попытки не переносятся.

**Откат.** Сначала прочитайте `schema-before.txt` из каталога резервной копии. При значении `t|f|t|t` прежняя версия уже совместима со схемой доски. Если новая версия ещё не переключена, можно запустить прежние службы. Если до релиза стояла версия без `000012_board_sync` и миграция прошла, прежний бинарник запускайте только после согласованного восстановления совместимой схемы из резервной копии. Down-миграция доски удаляет изображения и квитанции, поэтому автоматический откат БД исключён. Если переключение прервалось посередине, сначала сверьте фактические пути и возвращайте только заменённые компоненты.

Для отката совместимой версии после полного переключения используйте следующий блок с теми же переменными:

```bash
[[ "$(cat "$backup/schema-before.txt")" == 't|f|t|t' ]]
[[ "$(readlink "$app/frontend")" = "$release/frontend" ]]
[[ "$(readlink "$app/bin/edtech-server")" = "$release/bin/edtech-server" ]]
test -d "$app/frontend.before-$sha"
test -x "$app/bin/edtech-server.before-$sha"
test ! -e "$app/frontend.rejected-$sha"
test ! -e "$app/bin/edtech-server.rejected-$sha"
systemctl stop edtech-stage-go edtech-stage-django
mv -T "$app/frontend" "$app/frontend.rejected-$sha"
mv -T "$app/bin/edtech-server" "$app/bin/edtech-server.rejected-$sha"
mv -T "$app/frontend.before-$sha" "$app/frontend"
mv -T "$app/bin/edtech-server.before-$sha" "$app/bin/edtech-server"
systemctl start edtech-stage-django edtech-stage-go
curl -fsS https://edtech.promodetector.top/health
```

Расширенная схема и импорт остаются в БД. Восстановление из дампа или применение down-миграции решаются отдельно, так как могут удалить новые данные.

## 9. Открытые задачи и ограничения

- P0: провести полный тест с отдельными тестовыми аккаунтами учителя и ученика, привязать ученика через API, проверить черновик, назначение, ответы, доску, сдачу и изоляцию доступа.
- P0: перед реальными пользователями проверить авторизацию, CSRF, ограничения входа и запросов, загрузки файлов, отсутствие общих cookie родительского домена PromoRadar, сменить раскрытые SSH-учётные данные, ограничить доступ к тестовому стенду. HTTPS не заменяет аудит безопасности.
- P0/P1: автоматические резервные копии БД и uploads вне VPS с проверкой восстановления; контроль ресурсов и обновления ОС в согласованное окно.
- P1: подключить интерфейс приглашений и проверки работ, импортировать реальный банк с разрешёнными материалами, проверить статику и WebSocket, устранить предупреждения зависимостей.
- P1: согласовать восстановление интерфейсов календаря, CRM, статистики и комментариев без подмены реальных данных демонстрационными.
- P1: версионированный скрипт развёртывания под текущую схему, CI, отдельная тестовая БД, smoke-тесты, мониторинг.
- P2: календарь/аналитика/комментарии и другие функциональные блоки согласно согласованному MVP и roadmap; видеосвязь не реализована.

Итог: инфраструктурный запуск подтверждён журналами. Это тестовый стенд, не подтверждённый production-релиз. Все изменения приложения после первоначального развёртывания должны проходить ревью и тестирование.
