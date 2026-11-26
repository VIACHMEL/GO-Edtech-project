#!/usr/bin/env bash
set -Eeuo pipefail

cd "$(dirname "$0")"
root="$PWD"
fail() { echo "STOP: $*" >&2; exit 1; }
setup=false
case "${1:-}" in
    '') ;;
    --setup) setup=true ;;
    --help|-h) echo 'Использование: bash run.sh [--setup] (только локальная разработка)'; exit 0 ;;
    *) fail "Неизвестный аргумент: $1" ;;
esac
(($# <= 1)) || fail 'Слишком много аргументов'
case "$root/" in /opt/go-edtech-stage/*|/opt/go-edtech/*) fail 'На VPS используются службы systemd, не run.sh' ;; esac
command -v go >/dev/null || fail 'Не найден go'
command -v curl >/dev/null || fail 'Не найден curl'
windows=false
case "$(uname -s)" in MINGW*|MSYS*|CYGWIN*) windows=true ;; esac
venv="$root/frontend/.venv"
if $windows; then python="$venv/Scripts/python.exe"; binary="$root/bin/edtech-server.exe"
else python="$venv/bin/python"; binary="$root/bin/edtech-server"; fi
if $setup; then
    if [[ ! -x "$python" ]]; then
        if command -v python3 >/dev/null; then python3 -m venv "$venv"
        else python -m venv "$venv"; fi
    fi
    "$python" -m pip install -r "$root/frontend/requirements.txt"
    "$python" -m pip check
    (cd frontend/solve-board && npm ci --no-audit --no-fund && npm run build)
fi
[[ -x "$python" ]] || fail 'Подготовьте окружение: bash run.sh --setup'
test -s frontend/static/solve-board/dist/solve-board.js || fail 'Соберите доску: bash run.sh --setup'

# Не разрешаем dev-серверу случайно подключаться к удалённой/серверной БД.
if [[ -n "${DATABASE_URL:-}" ]]; then
    export DATABASE_URL
    "$python" - <<'PY'
import os
from urllib.parse import parse_qs, unquote, urlsplit
u = urlsplit(os.environ['DATABASE_URL'])
if (u.scheme not in ('postgres', 'postgresql') or u.hostname not in ('127.0.0.1', 'localhost', '::1')
        or unquote(u.path).strip('/') in ('', 'edtech_stage_db')
        or any(k.lower() in ('host', 'hostaddr', 'port', 'dbname', 'service') for k in parse_qs(u.query, keep_blank_values=True))):
    raise SystemExit('Требуется URL отдельной локальной БД (loopback), не VPS')
PY
elif $windows; then
    fail 'В Git Bash задайте DATABASE_URL отдельной локальной PostgreSQL (например Docker); см. docs/quick-vps-release.md'
fi
# Порты фиксированы, оба процесса доступны только на loopback.
"$python" - <<'PY'
import socket
for port in (8080, 8000):
    with socket.socket() as s:
        try:
            s.bind(('127.0.0.1', port))
        except OSError:
            raise SystemExit(f'Порт {port} занят; остановите предыдущий локальный запуск')
PY

go_pid=; django_pid=; started_db=false
cleanup() {
    rc=$?
    trap - EXIT INT TERM
    for pid in "$go_pid" "$django_pid"; do
        if [[ -n "$pid" ]]; then kill "$pid" 2>/dev/null || true; fi
    done
    for pid in "$go_pid" "$django_pid"; do
        if [[ -n "$pid" ]]; then wait "$pid" 2>/dev/null || true; fi
    done
    if $started_db; then pg_ctl -D "$root/data/postgres" -m fast stop || true; fi
    exit "$rc"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

if [[ -z "${DATABASE_URL:-}" ]]; then
    for command in initdb pg_ctl psql createdb; do command -v "$command" >/dev/null || fail "Не найден $command"; done
    mkdir -p data/postgres-socket
    chmod 700 data/postgres-socket
    if [[ ! -f data/postgres/PG_VERSION ]]; then
        initdb -D data/postgres -U edtech_local -A trust --encoding=UTF8 --no-locale
    fi
    if ! pg_ctl -D data/postgres status >/dev/null 2>&1; then
        [[ "$root" != *"'"* ]] || fail 'Путь с одинарной кавычкой не поддерживается pg_ctl'
        pg_ctl -D data/postgres -l "$root/data/postgres/server.log" \
            -o "-k '$root/data/postgres-socket' -c listen_addresses='' -p 15439" start
        started_db=true
    fi
    unset PGHOSTADDR PGSERVICE PGSERVICEFILE PGOPTIONS PGDATABASE
    export PGHOST="$root/data/postgres-socket" PGPORT=15439 PGUSER=edtech_local
    if [[ "$(psql -X -v ON_ERROR_STOP=1 -d postgres -Atc "SELECT 1 FROM pg_database WHERE datname='edtech_dev'")" != 1 ]]; then
        createdb edtech_dev
    fi
    # Применяем миграции только если в public совсем нет пользовательских таблиц.
    if [[ "$(psql -X -v ON_ERROR_STOP=1 -d edtech_dev -Atc "SELECT count(*) FROM pg_tables WHERE schemaname='public'")" == 0 ]]; then
        files=()
        for file in backend/internal/db/migrations/*.up.sql; do files+=(-f "$file"); done
        psql -X -d edtech_dev -v ON_ERROR_STOP=1 --single-transaction "${files[@]}"
    else
        echo 'Существующая локальная БД сохранена: миграции проверяются и применяются отдельно оператором.'
    fi
    export DATABASE_URL="$("$python" - "$PGHOST" <<'PY'
import sys
from urllib.parse import urlencode
print('postgres:///edtech_dev?' + urlencode(dict(host=sys.argv[1], port=15439, user='edtech_local', sslmode='disable')))
PY
)"
fi
# Внешняя локальная БД тоже используется без автоматических миграций.
export BACKEND_ADDR=127.0.0.1:8080 GO_API_BASE_URL=http://127.0.0.1:8080
export BOARD_ORIGIN=http://127.0.0.1:8080 DJANGO_PROXY_URL=http://127.0.0.1:8000
export DJANGO_SETTINGS_MODULE=config.settings DJANGO_DEBUG=true COOKIE_SECURE=false
export DJANGO_ALLOWED_HOSTS=localhost,127.0.0.1
export UPLOAD_DIR="${UPLOAD_DIR:-$root/data/uploads}" DATA_ROOT="${DATA_ROOT:-$root/data/raw}"
mkdir -p bin "$UPLOAD_DIR" "$DATA_ROOT"
(cd frontend && "$python" manage.py check)
(cd backend && go build -o "$binary" ./cmd/server)
"$binary" &
go_pid=$!
(cd frontend && exec "$python" manage.py runserver 127.0.0.1:8000 --noreload --insecure) &
django_pid=$!
ready=false
for ((i=0; i<30; i++)); do
    kill -0 "$go_pid" "$django_pid" 2>/dev/null || fail 'Один из серверов завершился'
    if curl -fsS --max-time 2 http://127.0.0.1:8080/health >/dev/null 2>&1 &&
       [[ "$(curl -sS --max-time 2 -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/login/ 2>/dev/null)" == 200 ]]; then
        ready=true; break
    fi
    sleep 1
done
$ready || fail 'Не дождались готовности Go и Django'
echo 'Сайт: http://127.0.0.1:8080/login/ — остановка: Ctrl+C'
wait -n "$go_pid" "$django_pid"
