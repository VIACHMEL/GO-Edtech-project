#!/usr/bin/env bash
set -euo pipefail

# Работаем из корня проекта, даже если скрипт вызван из другой папки.
cd "$(dirname "$0")"
root="$PWD"
python="$root/frontend/.venv/bin/python"

command -v go >/dev/null || { echo "Не найдена команда: go"; exit 1; }
[[ -x "$python" ]] || { echo "Сначала подготовьте frontend/.venv по frontend/README.md"; exit 1; }

# Проверяем порты до запуска, чтобы не оставить лишние процессы.
"$python" - <<'PY'
import socket
for port in (8080, 8000):
    with socket.socket() as connection:
        connection.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        try:
            connection.bind(('127.0.0.1', port))
        except OSError:
            raise SystemExit(f'Порт {port} занят. Сначала остановите предыдущий сервер.')
PY

go_pid=""
django_pid=""
started_db=false
cleanup() {
    trap - EXIT INT TERM
    # Останавливаем только процессы, запущенные этим скриптом.
    if [[ -n "$go_pid" ]]; then kill "$go_pid" 2>/dev/null || true; fi
    if [[ -n "$django_pid" ]]; then kill "$django_pid" 2>/dev/null || true; fi
    wait 2>/dev/null || true
    if $started_db; then
        pg_ctl -D "$root/data/postgres" -m fast stop
    fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# При заданном DATABASE_URL используем вашу готовую БД без миграций.
if [[ -z "${DATABASE_URL:-}" ]]; then
    for command in initdb pg_ctl psql createdb; do
        command -v "$command" >/dev/null || { echo "Не найдена команда: $command"; exit 1; }
    done
    mkdir -p data/postgres-socket
    chmod 700 data/postgres-socket
    if [[ ! -f data/postgres/PG_VERSION ]]; then
        initdb -D data/postgres -U edtech_local -A trust --encoding=UTF8 --no-locale
    fi
    if ! pg_ctl -D data/postgres status >/dev/null 2>&1; then
        pg_ctl -D data/postgres -l "$root/data/postgres/server.log" \
            -o "-k '$root/data/postgres-socket' -c listen_addresses='' -p 15439" start
        started_db=true
    fi
    export PGHOST="$root/data/postgres-socket" PGPORT=15439 PGUSER=edtech_local
    if [[ "$(psql -d postgres -Atc "SELECT 1 FROM pg_database WHERE datname = 'edtech_dev'")" != 1 ]]; then
        createdb edtech_dev
    fi
    if [[ "$(psql -d edtech_dev -Atc "SELECT to_regclass('public.tasks') IS NOT NULL")" == f ]]; then
        # Все миграции одной транзакцией: при ошибке изменения откатятся.
        files=()
        for file in backend/internal/db/migrations/*.up.sql; do
            files+=(-f "$file")
        done
        psql -d edtech_dev -v ON_ERROR_STOP=1 --single-transaction "${files[@]}"
    fi
    export DATABASE_URL="postgres:///edtech_dev?host=$root/data/postgres-socket&port=15439&user=edtech_local&sslmode=disable"
fi

export BACKEND_ADDR="${BACKEND_ADDR:-127.0.0.1:8080}"
export GO_API_BASE_URL=http://127.0.0.1:8080
export BOARD_ORIGIN="${BOARD_ORIGIN:-http://127.0.0.1:8080}"
export DJANGO_PROXY_URL=http://127.0.0.1:8000
export UPLOAD_DIR="${UPLOAD_DIR:-$root/data/uploads}"

# Запускаем собранный Go, чтобы Ctrl+C остановил именно сервер, без дочернего go run.
mkdir -p bin
(cd backend && go build -o "$root/bin/edtech-server" ./cmd/server)
bin/edtech-server &
go_pid=$!
(cd frontend && exec "$python" manage.py runserver 127.0.0.1:8000 --noreload --insecure) &
django_pid=$!

echo "Сайт: http://127.0.0.1:8080/login/ — остановка: Ctrl+C"
# Если один сервер завершится, завершаем и второй.
wait -n "$go_pid" "$django_pid"
