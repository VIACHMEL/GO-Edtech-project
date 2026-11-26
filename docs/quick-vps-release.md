# Быстрый выпуск EdTech на VPS (Git Bash / Bash)

Для разработчиков и ИИ-помощников. Обычное обновление после завершённой выкладки PR #25 — **три действия: PR → архив → выкладка**. Локальную ветку main переключать и делать git pull после каждого PR не нужно. Долгий регламент: [deployment-vps.md](deployment-vps.md). Эта инструкция не утверждает, что PR #25 уже успешно развёрнут: первый запуск ниже проверяет это.

## 0. Правила

- GitHub: VIACHMEL/GO-Edtech-project. Все команды локально — Git Bash (Windows) или Bash (Linux).
- VPS: root@201.8.94.39, сайт https://edtech.promodetector.top/. Только уполномоченный оператор с собственным SSH-доступом. Пароли и ключи не отправлять в чат или Git. IP уже указан в существующем deploy.sh.
- Текущий стенд: /opt/go-edtech-stage, edtech-stage-go.service, edtech-stage-django.service, PostgreSQL edtech_stage_db, роль edtech_stage_app, /etc/edtech-stage/app.env, /var/lib/edtech-stage/{uploads,task-data}.
- **Не запускать старые deploy.sh и run.sh на VPS:** они рассчитаны на другую схему. Не использовать rsync --delete, git reset --hard, force push, DROP DATABASE, не менять Caddy, PromoRadar и другие приложения.
- Не менять локальную ветку без разрешения; не создавать коммиты, PR и push без поручения. Проверить собственную Git-идентичность; не подменять автора и не присваивать чужие изменения.
- Не коммитить .env, секреты, дампы, скачанные задания, selected.tsv, пользовательские файлы, .venv, node_modules, сборки. Код и SQL-миграции коммитятся.
- Любая новая миграция, изменение systemd/конфигурации, импорт банка или несовместимое изменение данных требует отдельного согласованного плана. Обычный релиз не выполняет эти операции.
- Перед выкладкой: согласовать короткое окно обслуживания; проверить совместимость старого и нового кода с одной схемой БД, в том числе формат записываемых данных. После перезапуска Go пользователям потребуется повторный вход. Не выдавать локальные тесты за проверку VPS.

## 1. Отправить изменения и слить PR

В терминале в корне локального репозитория:

~~~bash
git branch --show-current
git status --short
git config user.name
git config user.email
git fetch origin
git diff --check
git log --oneline --left-right HEAD...origin/main
~~~

Работать в согласованной feature-ветке, не в main. Перед коммитом выполнить проверки затронутых модулей (см. AGENTS.md): Go — gofmt, go test ./..., go vet ./... из backend; Django — manage.py check/test и pip check в настроенном окружении; для доски — предусмотренные проектом тесты и npm run build. При пересечении миграций/контрактов согласовать порядок и повторить проверки. Не скрывать непройденные тесты.

После проверки добавить только нужные файлы через git add -- с конкретными путями. Не использовать git add . вслепую. Затем:

~~~bash
git diff --cached --check
git diff --cached --stat
git diff --cached --name-status
git status --short
~~~

Просмотреть staged diff, убедиться, что рабочие файлы не забыты и секретов нет. Если изменения ещё не закоммичены, создать коммит:

~~~bash
git commit -m "feat: краткое описание изменения"
~~~

Если коммит уже существует, этот шаг пропустить. Отправить текущую согласованную ветку:

~~~bash
branch="$(git branch --show-current)"
if [[ -n "$branch" && "$branch" != main ]]; then
  git push -u origin "$branch"
else
  echo "STOP: ожидалась feature-ветка" >&2
fi
~~~

Создать PR через https://github.com/VIACHMEL/GO-Edtech-project/compare (base: main, compare: ваша ветка). При наличии авторизованного GitHub CLI можно использовать gh pr create --base main --head "$branch" --fill. Слияние — только после ревью и обязательных проверок, через Create a merge commit. Если ветка уже содержит другой PR, явно описать это и проверить его актуальный head; отдельное повторное слияние не требуется, если нужные коммиты действительно входят в итоговый main.

Если изменения только в документации и не влияют на runtime, выкладка на VPS не нужна. Не удалять рабочую ветку автоматически.

## 2. Передать утверждённый main на VPS

Выполнять в Git Bash на ноутбуке. Ни переключение на main, ни pull не нужны: git archive читает объекты из origin/main, независимо от рабочей ветки. Если после вашего PR в main появились непроверенные изменения, вместо origin/main используйте конкретный полный SHA результата слияния из GitHub. Обязательно проверить SHA и diff перед передачей.

~~~bash
git fetch origin &&
sha="$(git rev-parse origin/main)" &&
git log -1 --oneline "$sha" &&
git merge-base --is-ancestor "$sha" origin/main
~~~

Проверьте напечатанный коммит, затем:

~~~bash
(
  set -euo pipefail
  [[ "$sha" =~ ^[0-9a-f]{40}$ ]]
  git merge-base --is-ancestor "$sha" origin/main
  archive="edtech-$sha.tar.gz"
  git archive --format=tar.gz --output="$archive" "$sha"
  gzip -t "$archive"
  [[ "$(git get-tar-commit-id < <(gzip -dc "$archive"))" == "$sha" ]]
  scp "$archive" "root@201.8.94.39:/root/$archive"
  printf 'Передан SHA: %s\n' "$sha"
)
~~~

Архив не добавлять в Git. Если нужен отдельный импорт КЕГЭ, selected.tsv передаётся через SCP отдельно, а не в составе git archive. Не импортировать банк повторно при редизайне.

## 3. Обычная выкладка без новых миграций — один блок

Продолжайте в том же Git Bash (переменная sha из шага 2). Вставьте **весь блок**. Он собирает новый релиз до остановки сайта, сохраняет БД и файлы, переключает только два сервиса EdTech и проверяет HTTPS. Старый релиз не удаляется.

Допустимо только после успешного первоначального релиза PR #25, когда стабильные пути frontend и Go уже являются ссылками на один каталог releases/<SHA>, установлены миграции 12 и 13 и настроен DATA_ROOT. Если это не так, сначала завершить или восстановить первоначальный релиз по длинной инструкции. При изменении хотя бы одного файла миграций скрипт остановится до остановки сайта; использовать раздел 4.

~~~bash
ssh root@201.8.94.39 "bash -s -- '$sha'" <<'REMOTE'
set -Eeuo pipefail
umask 022
export SYSTEMD_PAGER=cat
export PATH="/opt/edtech-toolchain/go/bin:/opt/edtech-toolchain/node/bin:$PATH"
cd /tmp
sha="$1"
[[ "$sha" =~ ^[0-9a-f]{40}$ ]]
app=/opt/go-edtech-stage
release="$app/releases/$sha"
archive="/root/edtech-$sha.tar.gz"
db=edtech_stage_db
envfile=/etc/edtech-stage/app.env
data=/var/lib/edtech-stage/task-data
front="$app/frontend"
bin="$app/bin/edtech-server"
url=https://edtech.promodetector.top
go_service=edtech-stage-go.service
django_service=edtech-stage-django.service

# Только один выпуск одновременно; все операторы должны соблюдать этот lock.
exec 9>/run/lock/edtech-stage-deploy.lock
flock -n 9 || { echo "STOP: другая выкладка"; exit 1; }

echo "=== Проверка текущего стенда ==="
systemctl is-active --quiet "$go_service"
systemctl is-active --quiet "$django_service"
systemctl show -p User -p WorkingDirectory -p ExecStart -p EnvironmentFiles \
  "$go_service" "$django_service"
[[ "$(systemctl show -p User --value "$go_service")" == edtechstage ]]
[[ "$(systemctl show -p User --value "$django_service")" == edtechstage ]]
[[ "$(systemctl show -p WorkingDirectory --value "$go_service")" == "$app" ]]
[[ "$(systemctl show -p WorkingDirectory --value "$django_service")" == "$front" ]]
systemctl show -p ExecStart --value "$go_service" | grep -Fq "$bin"
systemctl show -p ExecStart --value "$django_service" | grep -Fq "$front/.venv/bin/gunicorn"
test -f "$envfile"
test -f "$front/config/edtech_vps_settings.py"
test -L "$front"
test -L "$bin"
old_front="$(readlink -f "$front")"
old_bin="$(readlink -f "$bin")"
old_release="$(dirname "$(dirname "$old_bin")")"
[[ "$old_release" == "$app/releases/"* ]]
[[ "$(basename "$old_release")" =~ ^[0-9a-f]{40}$ ]]
[[ "$old_front" == "$old_release/frontend" ]]
[[ "$old_bin" == "$old_release/bin/edtech-server" ]]
test -x "$old_bin"
test -d "$old_release/backend/internal/db/migrations"
test -d /var/lib/edtech-stage/uploads
runuser -u edtechstage -- test -w "$data"
# Проверяем DATABASE_URL и фактический DATA_ROOT работающего Go,
# не печатая секретов из окружения процесса.
pid="$(systemctl show -p MainPID --value "$go_service")"
[[ "$pid" =~ ^[1-9][0-9]*$ ]]
[[ "$(readlink -f "/proc/$pid/exe")" == "$old_bin" ]]
python3 - "$pid" "$db" "$data" <<'PY'
import sys
from urllib.parse import unquote, urlsplit
pid, db, root = sys.argv[1:]
with open("/proc/" + pid + "/environ", "rb") as f:
    env = dict(x.split(b"=", 1) for x in f.read().split(b"\0") if b"=" in x)
dsn = urlsplit(env[b"DATABASE_URL"].decode())
assert dsn.scheme in ("postgres", "postgresql")
assert dsn.hostname in ("localhost", "127.0.0.1", "::1")
assert unquote(dsn.path) == "/" + db
assert env.get(b"DATA_ROOT", b"").decode() == root
print("OK: DATABASE_URL, DATA_ROOT")
PY
schema="$(runuser -u postgres -- psql -XAt -d "$db" -v ON_ERROR_STOP=1 -c "
SELECT EXISTS(SELECT FROM information_schema.columns
 WHERE table_schema='public' AND table_name='worksheets' AND column_name='revision'),
 EXISTS(SELECT FROM information_schema.columns
 WHERE table_schema='public' AND table_name='task_versions' AND column_name='condition_html'),
 to_regclass('public.board_files') IS NOT NULL,
 EXISTS(SELECT FROM information_schema.columns
 WHERE table_schema='public' AND table_name='board_operations' AND column_name='payload_digest');")"
[[ "$schema" == 't|t|t|t' ]]
df -h "$app" /var/lib/edtech-stage /var/backups/edtech-stage
free -h
gzip -t "$archive"
[[ "$(git get-tar-commit-id < <(gzip -dc "$archive"))" == "$sha" ]]
if [[ "$old_release" == "$release" ]]; then
  curl -fsS "$url/health"
  echo "OK: этот SHA уже запущен; повторная выкладка не нужна"
  exit 0
fi
# При неудачной предыдущей сборке каталог оставляем для диагностики.
test ! -e "$release"
test ! -L "$release"
install -d -m 0755 "$release/bin"
tar --no-same-owner -xzf "$archive" -C "$release"
if ! diff -qr "$old_release/backend/internal/db/migrations" \
              "$release/backend/internal/db/migrations"; then
  echo "STOP: изменились миграции. Сайт не остановлен. Нужен отдельный план."
  exit 1
fi

echo "=== Сборка нового релиза; старый сайт работает ==="
install -m 0644 "$old_front/config/edtech_vps_settings.py" \
  "$release/frontend/config/edtech_vps_settings.py"
chown -R edtechstage:edtechstage "$release"
runuser -u edtechstage -- env PATH="$PATH" bash -ec '
  cd "$1/backend"
  go build -p 2 -o "$1/bin/edtech-server" ./cmd/server
  go build -p 2 -o "$1/bin/kompege" ./cmd/kompege
  cd "$1/frontend/solve-board"
  npm ci --no-audit --no-fund
  npm run build
  cd ..
  python3 -m venv .venv
  .venv/bin/python -m pip install -r requirements.txt gunicorn==27.0.1
  .venv/bin/python -m pip check
' bash "$release"
for command in check collectstatic; do
  args=("$command")
  if [[ "$command" == collectstatic ]]; then args+=(--noinput); fi
  systemd-run --quiet --wait --pipe --collect --uid=edtechstage \
    --working-directory="$release/frontend" -p "EnvironmentFile=$envfile" \
    "$release/frontend/.venv/bin/python" manage.py "${args[@]}"
done
test -x "$release/bin/edtech-server"
test -x "$release/bin/kompege"
js="$release/frontend/staticfiles/solve-board/dist/solve-board.js"
test -s "$js"
caddy_user="$(systemctl show -p User --value caddy)"
runuser -u "${caddy_user:-root}" -- test -r "$js"
test ! -e "$front.next-$sha"
test ! -L "$front.next-$sha"
test ! -e "$bin.next-$sha"
test ! -L "$bin.next-$sha"

echo "=== Backup и короткое окно обслуживания ==="
umask 077
backup="$(mktemp -d "/var/backups/edtech-stage/release-${sha:0:12}-XXXXXXXX")"
printf '%s\n' "$old_front" "$old_bin" > "$backup/previous-paths.txt"
printf '%s\n' "$sha" > "$backup/target-sha.txt"
printf '%s\n' "$schema" > "$backup/schema-before.txt"
echo "Backup: $backup"
phase=backup
on_exit() {
  rc=$?
  if (( rc != 0 )); then
    echo "STOP: phase=$phase; backup=$backup; код=$rc" >&2
    if [[ "$phase" == backup ]]; then
      # Схема и ссылки ещё не менялись: можно вернуть прежние службы.
      systemctl start "$django_service" "$go_service" || true
    else
      echo "Проверьте состояние служб и ссылок; не повторяйте весь блок вслепую." >&2
    fi
  fi
}
trap on_exit EXIT
systemctl stop "$go_service" "$django_service"
runuser -u postgres -- pg_dump -Fc -d "$db" > "$backup/db.dump"
pg_restore -l "$backup/db.dump" > "$backup/db.list"
test -s "$backup/db.list"
pg_restore -f /dev/null "$backup/db.dump"
cp -a "$envfile" "$backup/app.env"
tar -cpf "$backup/files.tar" -C /var/lib/edtech-stage uploads task-data
tar -tf "$backup/files.tar" > "$backup/files.list"
test -s "$backup/files.list"
# Записанный backup — не полноценная проверка восстановления.
# Без согласованной миграции БД не меняем.

echo "=== Переключение ==="
phase=switch
umask 022
ln -sT "$release/frontend" "$front.next-$sha"
mv -Tf "$front.next-$sha" "$front"
ln -sT "$release/bin/edtech-server" "$bin.next-$sha"
mv -Tf "$bin.next-$sha" "$bin"
[[ "$(readlink -f "$front")" == "$release/frontend" ]]
[[ "$(readlink -f "$bin")" == "$release/bin/edtech-server" ]]
phase=start
systemctl start "$django_service" "$go_service"
systemctl is-active --quiet "$django_service"
systemctl is-active --quiet "$go_service"

echo "=== Проверка HTTPS ==="
phase=smoke
health="$(curl --retry 5 --retry-all-errors --retry-delay 2 --max-time 10 -fsS "$url/health")"
echo "$health"
grep -Eq '"status"[[:space:]]*:[[:space:]]*"ok"' <<< "$health"
[[ "$(curl -fsS -o /dev/null -w '%{http_code}' "$url/login/")" == 200 ]]
[[ "$(curl -fsS "$url/static/solve-board/dist/solve-board.js" | sha256sum | cut -d" " -f1)" == "$(sha256sum "$js" | cut -d" " -f1)" ]]
pid="$(systemctl show -p MainPID --value "$go_service")"
[[ "$(readlink -f "/proc/$pid/exe")" == "$release/bin/edtech-server" ]]
systemctl is-active --quiet postgresql
systemctl is-active --quiet caddy
systemctl --failed --no-pager
free -h
df -h "$app"
printf 'PASS: SHA=%s; backup=%s; HTTPS=%s\n' "$sha" "$backup" "$url"
phase=done
REMOTE
~~~

**Готовность:** строка PASS означает успешную сборку, переключение, проверки служб и HTTP/статики, но не полный E2E. В браузере проверить вход, страницу конструктора, создание черновика, назначение, сохранение/сдачу, доску и материалы — прежде всего затронутые функции. Проверить работоспособность остальных приложений отдельно; не перезапускать их.

## 4. Если новая миграция, новый импорт или другой стенд

Быстрый блок намеренно НЕ применяет SQL. Не повторять 000012/000013, если они уже применены. В репозитории нет подтверждённого универсального автоматического журнала миграций для этого стенда; определить фактическую схему и применённые версии по серверу и утверждённому регламенту, не по номеру файла или догадке.

1. Сверить фактическую схему, целевой SHA, порядок миграций и права роли приложения. Проверить миграции на копии БД. При изменении формата данных согласовать совместимость и план восстановления.
2. Собрать релиз отдельно до остановки. Сохранить PostgreSQL и все хранилища в окно обслуживания, проверить архивы. Применять только согласованные отсутствующие миграции через psql с ON_ERROR_STOP и транзакцией, если SQL поддерживает транзакционный запуск.
3. При необходимости импортировать новые задания отдельной службой systemd-run с EnvironmentFile=/etc/edtech-stage/app.env и DATA_ROOT=/var/lib/edtech-stage/task-data. Не передавать DATABASE_URL в аргументах и не выводить секреты. Сохранить полный source_ref, включая суффиксы :20/:21; проверять конкретный манифест, версии, число файлов и SHA-256. Уважать ограничения источника и не обходить 403/429.
4. Для первоначального выпуска банка 270/158 использовать раздел «Релиз банка ЕГЭ по информатике» в [deployment-vps.md](deployment-vps.md). Он исторически описывает два PR; фактически PR #25 содержит интегрированный PR #24, поэтому отдельный merge #24 не нужен, если его коммит уже в main. Сверить фактический результат VPS, не повторять импорт уже проверенных задач.

При изменении состава миграций быстрый блок успеет распаковать новый каталог releases/<SHA>, но не остановит сайт. Не удалять каталог без согласования; используйте его для контролируемой сборки по отдельному плану. После успешной миграции обновить регламент и зафиксировать фактическую версию.

## 5. Если SSH оборвался или появилась ошибка

Не повторять весь блок автоматически и не удалять ничего. Подключиться отдельно:

~~~bash
ssh root@201.8.94.39
~~~

~~~bash
systemctl status edtech-stage-go edtech-stage-django --no-pager
journalctl -u edtech-stage-go -u edtech-stage-django -n 80 --no-pager
readlink -f /opt/go-edtech-stage/frontend
readlink -f /opt/go-edtech-stage/bin/edtech-server
ls -dt /var/backups/edtech-stage/release-* 2>/dev/null | head
flock -n /run/lock/edtech-stage-deploy.lock true && echo "Lock свободен"
curl -fsS https://edtech.promodetector.top/health
~~~

При обрыве SSH обычный блок может прерваться, в отличие от отдельного systemd-run импорта. Сначала выяснить, продолжается ли операция, где остановилась и какие ссылки сейчас активны. Для длительного ручного обслуживания можно использовать уже установленный tmux; не устанавливать новые пакеты без согласования.

Если сбой до переключения и схема не менялась, после проверки можно запустить старые службы. Если переключение выполнено частично или smoke не прошёл, старые пути записаны в backup/previous-paths.txt. Восстанавливать только ссылки, не данные, и только после подтверждения совместимости старого кода с текущей БД. При несовместимой миграции старый бинарник запускать нельзя.

Пример ручного отката обычного релиза без изменения схемы (только с согласия оператора; указать реальный каталог backup, проверить пути и совместимость):

~~~bash
(
  set -euo pipefail
  cd /tmp
  exec 9>/run/lock/edtech-stage-deploy.lock
  flock -n 9
  read -rp "Путь к backup неудачного релиза: " backup
  [[ "$backup" == /var/backups/edtech-stage/release-* ]]
  mapfile -t previous < "$backup/previous-paths.txt"
  [[ "${#previous[@]}" == 2 ]]
  app=/opt/go-edtech-stage
  old_front="${previous[0]}"
  old_bin="${previous[1]}"
  old_release="$(dirname "$(dirname "$old_bin")")"
  [[ "$old_release" == "$app/releases/"* ]]
  [[ "$(basename "$old_release")" =~ ^[0-9a-f]{40}$ ]]
  [[ "$old_front" == "$old_release/frontend" ]]
  test -d "$old_front"
  test -x "$old_bin"
  test -L "$app/frontend"
  test -L "$app/bin/edtech-server"
  read -rp "Схема не менялась и совместимость проверена? Введите YES: " ok
  [[ "$ok" == YES ]]
  systemctl stop edtech-stage-go edtech-stage-django
  ln -sT "$old_front" "$app/frontend.rollback-$$"
  mv -Tf "$app/frontend.rollback-$$" "$app/frontend"
  ln -sT "$old_bin" "$app/bin/edtech-server.rollback-$$"
  mv -Tf "$app/bin/edtech-server.rollback-$$" "$app/bin/edtech-server"
  systemctl start edtech-stage-django edtech-stage-go
  curl --retry 5 --retry-all-errors --retry-delay 2 --max-time 10 -fsS \
    https://edtech.promodetector.top/health
)
~~~

Не использовать этот блок при изменении схемы или несовместимой записи новых данных. Восстановление дампа, удаление данных или down-миграция — отдельное решение владельца. Резервные копии с секретами остаются root-only; не отправлять их в Git и не публиковать.

## 6. Что должен сообщить ИИ после работы

- Номер PR, результат проверок, полный SHA слитого и фактически запущенного релиза (не предполагать их равенство).
- Был ли изменён backend/frontend, были ли миграции, импорт, изменённые контракты.
- Путь резервной копии; результаты HTTP, статических файлов, служб и ручных сценариев.
- Ошибки/предупреждения и непройденные проверки. Не заявлять о полном развёртывании, если есть только Git push или локальная сборка.
- Не останавливаться после каждого успешного шага: продолжать до целостного результата в рамках согласованного доступа. При блокирующей ошибке остановить опасные операции и сообщить конкретное состояние.

Документ составлен по процедуре выпуска PR #25 и текущей структуре репозитория. Блок быстрого выпуска требует пробного запуска уполномоченным оператором; его работа на реальном VPS ещё не подтверждена. Он не устанавливает новый CI/CD и не меняет действующие службы до явного запуска.
