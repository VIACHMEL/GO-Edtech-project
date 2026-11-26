# EdTech: локальный запуск и быстрый релиз VPS

Для людей и ИИ-помощников. Все команды — Bash / Git Bash. Два существующих скрипта имеют разные назначения:

| Команда | Где | Что делает |
|---|---|---|
| bash run.sh --setup | Компьютер | Подготавливает Python и доску, запускает локальный Go и Django |
| bash run.sh | Компьютер | Запускает ранее подготовленную локальную версию |
| bash deploy.sh --plan | Компьютер | Получает origin/main и показывает целевой SHA, не подключаясь к VPS |
| bash deploy.sh | Компьютер | После подтверждения собирает, резервирует и обновляет HTTPS-стенд |
| bash deploy.sh --sha FULL_SHA --yes | Компьютер | Выкладывает конкретный согласованный коммит из истории origin/main без повторного вопроса |

Прежний подробный регламент: [deployment-vps.md](deployment-vps.md). Он содержит исторические шаги первого релиза и проверки банка. На VPS нет необходимости выполнять run.sh или git pull.

## 1. Локальная разработка

Нужны Go, Python 3.12–3.14, Bash, curl, Node.js и npm. Для Windows используется Git Bash и frontend/.venv/Scripts/python.exe; для Linux/WSL — frontend/.venv/bin/python. В Git Bash на Windows нужна подготовленная отдельная локальная PostgreSQL по TCP; Unix-сокет локального кластера Windows не используется.

При первом запуске или изменении зависимостей/исходников доски:

~~~bash
bash run.sh --setup
~~~

В последующие разы:

~~~bash
bash run.sh
~~~

Открыть http://127.0.0.1:8080/login/, остановка Ctrl+C. Go и Django слушают 127.0.0.1:8080 и :8000. Скрипт проверяет порты, готовность /health и /login/, завершает только запущенные им процессы.

Без DATABASE_URL на Linux/WSL используется локальный кластер data/postgres с Unix-сокетом и отключённым TCP. Установленные initdb, pg_ctl, psql, createdb должны быть в PATH; запускать не от root. Все up-миграции применяются одной транзакцией только к пустой public-схеме. Если БД уже существует, автоматических изменений схемы нет. Перед изменением схемы проверить текущие миграции и сделать копию. При внешней локальной БД скрипт также никогда не применяет миграции автоматически.

Для Windows/Git Bash, если отдельная БД уже подготовлена, перед запуском экспортировать её URL:

~~~bash
export DATABASE_URL='postgresql://LOCAL_USER:LOCAL_PASSWORD@127.0.0.1:15432/LOCAL_DB?sslmode=disable'
bash run.sh --setup
~~~

Заменить LOCAL_* своими локальными значениями; не использовать пароль VPS. URL хранить только в локальном окружении. .env автоматически не загружается. Проверка run.sh допускает только локальный TCP URL, исключает edtech_stage_db и параметры, перенаправляющие соединение на другой host/port/dbname. Она не заменяет контроль фактического подключения: не использовать SSH-туннель до рабочей БД.

Если БД ещё нет, пример нового отдельного Docker Compose проекта из корня репозитория (порт и пароль ниже только для локальной разработки; при занятом порте выбрать свободный):

~~~bash
export POSTGRES_USER=edtech_local POSTGRES_PASSWORD=edtech_local_dev_only
export POSTGRES_DB=edtech_local_dev POSTGRES_PORT=15439
docker compose -p edtech-local up -d postgres
docker compose -p edtech-local exec -T postgres pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB"
~~~

Продолжать только после готовности PostgreSQL. Для **новой пустой** БД выполнить:

~~~bash
(
    set -euo pipefail
    tables="$(docker compose -p edtech-local exec -T postgres psql -XAt -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -c "SELECT count(*) FROM pg_tables WHERE schemaname='public'")"
    [[ "$tables" == 0 ]] || { echo 'STOP: БД не пуста; миграции вручную'; exit 1; }
    for file in backend/internal/db/migrations/*.up.sql; do cat "$file"; printf '\n'; done |
        docker compose -p edtech-local exec -T postgres psql -X -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 --single-transaction -f -
)
~~~

Затем:

~~~bash
export DATABASE_URL="postgresql://$POSTGRES_USER:$POSTGRES_PASSWORD@127.0.0.1:$POSTGRES_PORT/$POSTGRES_DB?sslmode=disable"
bash run.sh --setup
~~~

Docker Compose БД остаётся запущенной после выхода run.sh: она управляется отдельно. Не выполнять down -v и не удалять существующие данные. Если уже есть рабочая локальная БД, использовать её и не повторять пример создания.

## 2. Отправить изменения в GitHub

Работать в согласованной feature-ветке. Не переключать ветку, не создавать коммит или PR без поручения, не использовать чужую Git-идентичность.

~~~bash
git branch --show-current
git status --short
git config user.name
git config user.email
git fetch origin
git diff --check
git log --oneline --left-right HEAD...origin/main
~~~

Выполнить предусмотренные AGENTS.md тесты изменённых модулей (Go, Django, доска). Добавить только нужные файлы по конкретным путям через git add --, проверить staged diff и отсутствие секретов, затем:

~~~bash
git diff --cached --check
git diff --cached --name-status
git commit -m "feat: краткое описание"
~~~

Если коммит уже создан, не повторять. Проверить имя ветки, после чего:

~~~bash
branch="$(git branch --show-current)"
if [[ -n "$branch" && "$branch" != main ]]; then
    git push -u origin "$branch"
else
    echo 'STOP: ожидалась feature-ветка' >&2
fi
~~~

Открыть PR в main: https://github.com/VIACHMEL/GO-Edtech-project/compare. После ревью и обязательных проверок — Create a merge commit. Если изменения включают другой PR, сверить его head и описать интеграцию. Не делать force push. Не отправлять базы, selected.tsv, скачанные задания, дампы, ключи, .env, данные пользователей, node_modules, .venv и сборки.

## 3. Обычный релиз: одна команда

Адрес стенда: https://edtech.promodetector.top/. SSH: root@201.8.94.39. Только уполномоченный оператор; ключи и пароли не отправлять в Git или чат. Обновляем только /opt/go-edtech-stage, edtech-stage-go.service и edtech-stage-django.service. Не затрагиваем другие сервисы, Caddy, PromoRadar, старый /opt/go-edtech и другие БД.

Один раз получить новую версию deploy.sh из слитого main. При нахождении в своей ветке не нужно переключаться на main: можно запустить скрипт непосредственно из утверждённого origin/main, оставаясь в корне рабочей копии:

~~~bash
git fetch origin && bash -c "$(git show origin/main:deploy.sh)" deploy.sh --plan
~~~

Для самой выкладки без переключения ветки аналогично выполнить `git fetch origin && bash -c "$(git show origin/main:deploy.sh)" deploy.sh` и подтвердить SHA. Если обновлённый deploy.sh уже есть в рабочей копии, обычный процесс после слияния PR:

~~~bash
bash deploy.sh --plan
bash deploy.sh
~~~

Проверить показанный SHA, согласовать окно обслуживания и совместимость данных, ввести YES. Если после нужного PR в main появились другие изменения, указать **полный SHA** проверенного слияния:

~~~bash
bash deploy.sh --sha FULL_SHA
~~~

--yes допустим только после явного согласования тестов, SHA, совместимости схемы/данных и окна обслуживания; не использовать ради обхода подтверждения. --plan обновляет remote refs через git fetch, но не меняет рабочие файлы и не обращается к VPS. При изменении только документации выкладка не нужна.

Что делает deploy.sh автоматически:

1. Проверяет origin, принадлежность SHA истории main, создаёт git archive только из этого коммита, проверяет SHA и SHA-256, передаёт архив через SCP под уникальным именем.
2. На VPS ставит flock, проверяет активные службы, их пользователя и пути, фактический DATABASE_URL, DATA_ROOT, контрольные признаки схемы 000012/000013, текущий релиз и файлы миграций. При различии миграций прекращает работу до остановки сайта.
3. Собирает Go и импортёр, доску и Django в отдельном releases/<SHA>; текущий сайт продолжает работать. Выполняет Django check/collectstatic и проверку прав Caddy.
4. Останавливает только EdTech, сохраняет PostgreSQL, app.env, uploads и task-data в root-only каталог /var/backups/edtech-stage/release-*. Проверяет чтение дампа и архивов; это ещё не тест восстановления.
5. Переключает ссылки на новую сборку и запускает обе службы; проверяет HTTPS /health, /login/, SHA-256 отдаваемого JS доски и путь реально запущенного Go-процесса. Проверяет Caddy и PostgreSQL.
6. Печатает PASS с SHA и путём резервной копии. Старые релизы, архивы и резервные копии на VPS не удаляет. Временный архив на компьютере удаляет только из собственной временной директории.

Сеансы Go хранятся в памяти: после переключения нужен повторный вход. Сборка занимает время, но сайт до backup остаётся доступен. Отсутствие ручных шагов не означает мгновенную сборку.

После PASS проверить браузером изменённые функции и основной сценарий преподаватель → конструктор → выдача → ученик → ответ → сдача; доску, изображения и вложения. Проверить другие приложения отдельно, не перезапускать их. PASS не доказывает полный E2E.

**Ограничения:** быстрый путь рассчитан на уже завершённый первый выпуск PR #25: обе стабильные ссылки должны вести в один releases/<SHA>, миграции 12/13 должны быть применены, DATA_ROOT настроен постоянным Environment= в Go-службе. При несоответствии он остановится. Не считать эту инструкцию доказательством, что первый релиз уже выполнен. Диск и объём накопленных backup контролировать отдельно; автоматической очистки нет.

## 4. Миграции и импорт — отдельный контролируемый выпуск

deploy.sh намеренно не применяет SQL, не пересоздаёт БД, не меняет конфигурации systemd и не импортирует банк повторно. Даже изменение старого файла миграции означает STOP до остановки сайта. Не пытаться обойти проверку редактированием файлов на VPS.

При новой миграции определить фактическую схему, проверить SQL на копии БД, согласовать окно и совместимость старого и нового кода, права роли edtech_stage_app и восстановление. Собрать код до остановки, сделать проверенный backup, применять только отсутствующие согласованные up-миграции с ON_ERROR_STOP и транзакцией, если SQL поддерживает транзакционный запуск. Миграции 12/13 не повторять, если уже применены. При несовместимой миграции запуск старого бинарника запрещён до согласованного восстановления схемы.

Для первого выпуска 270 заданий и 158 материалов — [раздел релиза банка](deployment-vps.md#релиз-банка-егэ-по-информатике). Он исторически описывает два PR; PR #25 уже содержит интегрированную ветку доски из PR #24. Не выполнять отдельный merge #24 повторно. selected.tsv передаётся через SCP отдельно, импортёр запускается через systemd-run с EnvironmentFile=/etc/edtech-stage/app.env и DATA_ROOT=/var/lib/edtech-stage/task-data. Сохранять source_ref целиком, включая :20/:21; сверять версии, квоты и контрольные суммы материалов. Не обходить ограничения источника 403/429.

При STOP из-за миграций новый releases/<SHA> уже может быть распакован. Не удалять и не перезаписывать его автоматически: использовать при согласованном ручном выпуске после проверки SHA и состояния. Не перезапускать скрипт вслепую.

## 5. Диагностика и ручной откат

При ошибке скрипт выводит phase, SHA, backup и archive. Если сбой произошёл до переключения ссылок в фазе backup, он пытается вернуть прежние службы. При проблеме после переключения требуется проверка состояния; автоматический откат не выполняется, чтобы не запускать неподходящий код поверх новых данных.

При обрыве SSH обычная сборка/выкладка может прерваться. Подключиться отдельно:

~~~bash
ssh root@201.8.94.39
~~~

~~~bash
systemctl status edtech-stage-go edtech-stage-django --no-pager
journalctl -u edtech-stage-go -u edtech-stage-django -n 80 --no-pager
readlink -f /opt/go-edtech-stage/frontend
readlink -f /opt/go-edtech-stage/bin/edtech-server
ls -dt /var/backups/edtech-stage/release-* 2>/dev/null | head
flock -n /run/lock/edtech-stage-deploy.lock true && echo 'Lock свободен'
curl -fsS https://edtech.promodetector.top/health
~~~

До принятия решения проверить, нет ли продолжающейся операции. Не удалять файлы, не запускать повторно миграции, не выполнять DROP DATABASE. Не выводить содержимое app.env.

Ручной откат обычного **совместимого** релиза: в backup/previous-paths.txt записаны прежние абсолютные пути. Следующий блок предназначен только для случая, когда текущая схема и записанные новой версией данные совместимы со старым кодом. При миграциях использовать отдельный план; он не восстанавливает БД и файлы из дампа.

~~~bash
(
    set -euo pipefail
    cd /tmp
    exec 9>/run/lock/edtech-stage-deploy.lock
    flock -n 9
    read -rp 'Путь backup неудачного релиза: ' backup
    [[ "$backup" == /var/backups/edtech-stage/release-* ]]
    test -s "$backup/db.dump"
    mapfile -t previous < "$backup/previous-paths.txt"
    [[ "${#previous[@]}" == 2 ]]
    app=/opt/go-edtech-stage
    old_front="${previous[0]}"; old_bin="${previous[1]}"
    old_release="$(dirname "$(dirname "$old_bin")")"
    [[ "$old_release" == "$app/releases/"* && "$(basename "$old_release")" =~ ^[0-9a-f]{40}$ ]]
    [[ "$old_front" == "$old_release/frontend" && "$old_bin" == "$old_release/bin/edtech-server" ]]
    test -d "$old_front"; test -x "$old_bin"
    test -L "$app/frontend"; test -L "$app/bin/edtech-server"
    read -rp 'Совместимость схемы и данных подтверждена? Введите YES: ' ok
    [[ "$ok" == YES ]]
    systemctl stop edtech-stage-go edtech-stage-django
    ln -sT "$old_front" "$app/frontend.rollback-$$"
    mv -Tf "$app/frontend.rollback-$$" "$app/frontend"
    ln -sT "$old_bin" "$app/bin/edtech-server.rollback-$$"
    mv -Tf "$app/bin/edtech-server.rollback-$$" "$app/bin/edtech-server"
    systemctl start edtech-stage-django edtech-stage-go
    systemctl is-active edtech-stage-django edtech-stage-go
    curl --retry 5 --retry-all-errors --retry-delay 2 --max-time 10 -fsS https://edtech.promodetector.top/health
)
~~~

Не восстанавливать дамп и не делать down-миграцию без отдельного разрешения. Резервные копии с секретами остаются root-only; тест восстановления и внешнее резервирование выполняются отдельно.

## 6. Инструкция для ИИ и результаты проверки

Прочитать AGENTS.md и этот файл. Проверить ветку, diff, локальные тесты, подготовить PR; после согласованного merge использовать bash deploy.sh --plan и bash deploy.sh либо утверждённый SHA. Не переключать локальные ветки без разрешения, не менять чужие сервисы, не удалять файлы. При обычном выпуске не повторять десятки ручных операций; продолжать до целостного результата, останавливать опасные действия при реальном блокере.

В отчёте указать PR, утверждённый и реально запущенный SHA, тесты, состояние служб, HTTPS/статики, путь backup и ручные проверки. Не считать Git push или локальный PASS доказательством выкладки.

Для данной версии обоих скриптов выполнены bash -n (включая удалённый блок), --help, тесты deploy.sh во временном Git-репозитории с подменёнными SSH/SCP: --plan, отмена, некорректный и неслитый SHA, создание архива и передача согласованного SHA/хеша. Проверено отклонение run.sh удалённого DSN, серверной БД и переопределённого hostaddr. Сверены blob SHA протестированных файлов с GitHub. Реальный запуск Go/Django, Windows Git Bash и VPS в этой проверке не выполнялись. Первый фактический запуск нужен для приёмки; обновление не запускалось автоматически.
