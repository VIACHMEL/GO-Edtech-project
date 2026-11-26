#!/usr/bin/env bash
set -Eeuo pipefail

# Запуск с компьютера разработчика, не из каталога приложения на VPS.
usage() {
    echo 'Использование: bash deploy.sh [--sha SHA] [--yes | --plan]'
    echo 'По умолчанию: проверенный origin/main; --plan только показывает SHA, --yes подтверждает выкладку.'
}
fail() { echo "STOP: $*" >&2; exit 1; }
requested=; yes=false; plan=false
while (($#)); do
    case "$1" in
        --sha) (($# >= 2)) || fail 'Нужен полный SHA'; requested="$2"; shift ;;
        --yes) yes=true ;;
        --plan) plan=true ;;
        --help|-h) usage; exit 0 ;;
        *) usage; fail "Неизвестный аргумент: $1" ;;
    esac
    shift
done
cd "$(git rev-parse --show-toplevel)"
case "$PWD/" in /opt/go-edtech-stage/*|/opt/go-edtech/*) fail 'Запускайте deploy.sh с компьютера разработчика' ;; esac
case "$(git remote get-url origin)" in
    https://github.com/VIACHMEL/GO-Edtech-project|https://github.com/VIACHMEL/GO-Edtech-project.git|git@github.com:VIACHMEL/GO-Edtech-project.git|git@github.com:VIACHMEL/GO-Edtech-project|ssh://git@github.com/VIACHMEL/GO-Edtech-project.git) ;;
    *) fail 'origin не указывает на командный репозиторий' ;;
esac
git fetch origin
sha="${requested:-$(git rev-parse origin/main)}"
[[ "$sha" =~ ^[0-9a-f]{40}$ ]] || fail 'Нужен полный SHA'
git merge-base --is-ancestor "$sha" origin/main || fail 'SHA не входит в origin/main'
git log -1 --oneline "$sha"
echo "Цель: root@201.8.94.39, SHA=$sha (локальные изменения не входят в архив)"
$plan && exit 0
if ! $yes; then
    read -rp 'Ревью, тесты, совместимость данных и окно обслуживания согласованы? Введите YES: ' answer
    [[ "$answer" == YES ]] || fail 'Выкладка отменена'
fi
for cmd in ssh scp gzip sha256sum; do command -v "$cmd" >/dev/null || fail "Нет $cmd"; done
tmp="$(mktemp -d)"
trap 'rm -f -- "$tmp/release.tar.gz"; rmdir -- "$tmp"' EXIT
archive="$tmp/release.tar.gz"
git archive --format=tar.gz --output="$archive" "$sha"
gzip -t "$archive"
[[ "$(git get-tar-commit-id < <(gzip -dc "$archive"))" == "$sha" ]] || fail 'SHA архива не совпал'
digest="$(sha256sum "$archive" | cut -d' ' -f1)"
# Уникальное имя исключает перезапись архива параллельным оператором.
remote="/root/edtech-$sha-$$-$RANDOM-$RANDOM.tar.gz"
scp "$archive" "root@201.8.94.39:$remote"
ssh -T root@201.8.94.39 "bash -s -- '$sha' '$remote' '$digest'" <<'REMOTE'
set -Eeuo pipefail
umask 022
export SYSTEMD_PAGER=cat
export PATH="/opt/edtech-toolchain/go/bin:/opt/edtech-toolchain/node/bin:$PATH"
cd /tmp
sha="$1"; archive="$2"; digest="$3"
[[ "$sha" =~ ^[0-9a-f]{40}$ && "$digest" =~ ^[0-9a-f]{64}$ ]]
[[ "$archive" =~ ^/root/edtech-[0-9a-f]{40}-[0-9]+-[0-9]+-[0-9]+\.tar\.gz$ ]]
app=/opt/go-edtech-stage
release="$app/releases/$sha"
front="$app/frontend"; bin="$app/bin/edtech-server"
db=edtech_stage_db; envfile=/etc/edtech-stage/app.env
data=/var/lib/edtech-stage/task-data
url=https://edtech.promodetector.top
go_service=edtech-stage-go.service; django_service=edtech-stage-django.service
phase=preflight; backup=
fail() { echo "STOP: $*" >&2; exit 1; }
on_exit() {
    rc=$?
    if ((rc != 0)); then
        echo "STOP: phase=$phase; SHA=$sha; backup=${backup:-none}; archive=$archive; code=$rc" >&2
        if [[ "$phase" == backup ]]; then
            # Ссылки и схема ещё не менялись.
            systemctl start "$django_service" "$go_service" || true
        fi
        echo 'Не повторяйте выкладку вслепую; см. docs/quick-vps-release.md.' >&2
    fi
}
trap on_exit EXIT
exec 9>/run/lock/edtech-stage-deploy.lock
flock -n 9 || fail 'Другая выкладка уже идёт'
[[ $EUID == 0 ]] || fail 'Нужен уполномоченный root-оператор'
for service in "$go_service" "$django_service"; do
    systemctl is-active --quiet "$service"
    [[ "$(systemctl show -p User --value "$service")" == edtechstage ]]
    [[ "$(systemctl show -p EnvironmentFiles --value "$service")" == "$envfile (ignore_errors=no)" ]]
done
[[ "$(systemctl show -p WorkingDirectory --value "$go_service")" == "$app" ]]
[[ "$(systemctl show -p WorkingDirectory --value "$django_service")" == "$front" ]]
systemctl show -p ExecStart --value "$go_service" | grep -F "$bin" >/dev/null
systemctl show -p ExecStart --value "$django_service" | grep -F "$front/.venv/bin/gunicorn" >/dev/null
test -f "$envfile"; test -L "$front"; test -L "$bin"
old_front="$(readlink -f "$front")"; old_bin="$(readlink -f "$bin")"
old_release="$(dirname "$(dirname "$old_bin")")"
[[ "$old_release" == "$app/releases/"* && "$(basename "$old_release")" =~ ^[0-9a-f]{40}$ ]]
[[ "$old_front" == "$old_release/frontend" && "$old_bin" == "$old_release/bin/edtech-server" ]]
test -x "$old_bin"; test -f "$old_front/config/edtech_vps_settings.py"
test -d "$old_release/backend/internal/db/migrations"
test -d /var/lib/edtech-stage/uploads
runuser -u edtechstage -- test -w "$data"
# Сверяем фактическое окружение работающего Go, не печатая секреты.
pid="$(systemctl show -p MainPID --value "$go_service")"
[[ "$pid" =~ ^[1-9][0-9]*$ && "$(readlink -f "/proc/$pid/exe")" == "$old_bin" ]]
python3 - "$pid" "$db" "$data" <<'PY'
import sys
from urllib.parse import parse_qs, unquote, urlsplit
pid, db, root = sys.argv[1:]
with open('/proc/' + pid + '/environ', 'rb') as f:
    env = dict(x.split(b'=', 1) for x in f.read().split(b'\0') if b'=' in x)
dsn = urlsplit(env[b'DATABASE_URL'].decode())
assert dsn.scheme in ('postgres', 'postgresql')
assert dsn.hostname in ('localhost', '127.0.0.1', '::1')
assert unquote(dsn.path) == '/' + db
assert not any(k.lower() in ('host', 'hostaddr', 'port', 'dbname', 'service') for k in parse_qs(dsn.query, keep_blank_values=True))
assert env.get(b'DATA_ROOT', b'').decode() == root
print('OK: database and storage')
PY
# Проверяем настройки следующего запуска, а не только старый процесс.
systemd-run --quiet --wait --pipe --collect --uid=edtechstage \
    -p "EnvironmentFile=$envfile" /usr/bin/python3 -c '
import os, sys
from urllib.parse import parse_qs, unquote, urlsplit
u = urlsplit(os.environ["DATABASE_URL"])
assert u.scheme in ("postgres", "postgresql")
assert u.hostname in ("localhost", "127.0.0.1", "::1")
assert unquote(u.path) == "/" + sys.argv[1]
assert not any(k.lower() in ("host", "hostaddr", "port", "dbname", "service") for k in parse_qs(u.query, keep_blank_values=True))
assert os.environ.get("DATA_ROOT", sys.argv[2]) == sys.argv[2]
' "$db" "$data"
systemctl show -p Environment --value "$go_service" | grep -E "(^|[[:space:]])DATA_ROOT=$data([[:space:]]|$)" >/dev/null || fail 'Нужно проверить постоянный DATA_ROOT в systemd'
schema="$(runuser -u postgres -- psql -XAt -d "$db" -v ON_ERROR_STOP=1 -c "
SELECT EXISTS(SELECT FROM information_schema.columns WHERE table_schema='public' AND table_name='worksheets' AND column_name='revision'),
 EXISTS(SELECT FROM information_schema.columns WHERE table_schema='public' AND table_name='task_versions' AND column_name='condition_html'),
 to_regclass('public.board_files') IS NOT NULL,
 EXISTS(SELECT FROM information_schema.columns WHERE table_schema='public' AND table_name='board_operations' AND column_name='payload_digest');")"
[[ "$schema" == 't|t|t|t' ]] || fail 'Стенд ещё не после миграций 12/13 или схема отличается'
[[ "$(sha256sum "$archive" | cut -d' ' -f1)" == "$digest" ]]
gzip -t "$archive"
[[ "$(git get-tar-commit-id < <(gzip -dc "$archive"))" == "$sha" ]]
if [[ "$old_release" == "$release" ]]; then
    curl -fsS --max-time 10 "$url/health"
    echo "OK: SHA=$sha уже запущен; ничего не меняли"; exit 0
fi
# Не перезаписываем предыдущие сборки/неудачные попытки.
test ! -e "$release"; test ! -L "$release"
df -h "$app" /var/lib/edtech-stage /var/backups/edtech-stage
free -h
phase=build
install -d -m 0755 "$release/bin"
tar --no-same-owner -xzf "$archive" -C "$release"
diff -qr "$old_release/backend/internal/db/migrations" "$release/backend/internal/db/migrations" || fail 'Изменились миграции; нужен отдельный план, сайт не остановлен'
install -m 0644 "$old_front/config/edtech_vps_settings.py" "$release/frontend/config/edtech_vps_settings.py"
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
test -x "$release/bin/edtech-server"; test -x "$release/bin/kompege"
js="$release/frontend/staticfiles/solve-board/dist/solve-board.js"
test -s "$js"
caddy_user="$(systemctl show -p User --value caddy)"
runuser -u "${caddy_user:-root}" -- test -r "$js"
for path in "$front.next-$sha" "$bin.next-$sha"; do test ! -e "$path"; test ! -L "$path"; done
# Повторно проверяем, что текущие ссылки не изменились во время сборки.
[[ "$(readlink -f "$front")" == "$old_front" && "$(readlink -f "$bin")" == "$old_bin" ]]
phase=backup
umask 077
backup="$(mktemp -d "/var/backups/edtech-stage/release-${sha:0:12}-XXXXXXXX")"
printf '%s\n' "$old_front" "$old_bin" > "$backup/previous-paths.txt"
printf '%s\n' "$sha" > "$backup/target-sha.txt"
printf '%s\n' "$schema" > "$backup/schema-before.txt"
echo "Backup: $backup"
systemctl stop "$go_service" "$django_service"
runuser -u postgres -- pg_dump -Fc -d "$db" > "$backup/db.dump"
pg_restore -l "$backup/db.dump" > "$backup/db.list"
test -s "$backup/db.list"
pg_restore -f /dev/null "$backup/db.dump"
cp -a "$envfile" "$backup/app.env"
tar -cpf "$backup/files.tar" -C /var/lib/edtech-stage uploads task-data
tar -tf "$backup/files.tar" > "$backup/files.list"
test -s "$backup/files.list"
# Никаких миграций, импорта или изменения конфигурации systemd.
phase=switch
umask 022
ln -sT "$release/frontend" "$front.next-$sha"
mv -Tf "$front.next-$sha" "$front"
ln -sT "$release/bin/edtech-server" "$bin.next-$sha"
mv -Tf "$bin.next-$sha" "$bin"
[[ "$(readlink -f "$front")" == "$release/frontend" && "$(readlink -f "$bin")" == "$release/bin/edtech-server" ]]
phase=start
systemctl start "$django_service" "$go_service"
systemctl is-active --quiet "$django_service"
systemctl is-active --quiet "$go_service"
phase=smoke
health="$(curl --retry 5 --retry-all-errors --retry-delay 2 --max-time 10 -fsS "$url/health")"
echo "$health"
grep -Eq '"status"[[:space:]]*:[[:space:]]*"ok"' <<< "$health"
[[ "$(curl --max-time 15 -fsS -o /dev/null -w '%{http_code}' "$url/login/")" == 200 ]]
[[ "$(curl --max-time 30 -fsS "$url/static/solve-board/dist/solve-board.js" | sha256sum | cut -d' ' -f1)" == "$(sha256sum "$js" | cut -d' ' -f1)" ]]
pid="$(systemctl show -p MainPID --value "$go_service")"
[[ "$(readlink -f "/proc/$pid/exe")" == "$release/bin/edtech-server" ]]
systemctl is-active --quiet postgresql
systemctl is-active --quiet caddy
systemctl --failed --no-pager
free -h
df -h "$app"
phase=done
printf 'PASS: SHA=%s; backup=%s; HTTPS=%s\n' "$sha" "$backup" "$url"
REMOTE
