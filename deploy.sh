#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

server=root@201.8.94.39

# Код обновляется отдельно от .env, окружения Python и пользовательских данных.
ssh -p 22 "$server" 'mkdir -p /opt/go-edtech'
rsync -az --delete -e 'ssh -p 22' \
    --exclude=.env --exclude='.env.*' --exclude=.venv --exclude=__pycache__ \
    --exclude=node_modules --exclude=dist --exclude=staticfiles \
    backend frontend run.sh "$server:/opt/go-edtech/"

ssh -p 22 "$server" 'bash -se' <<'REMOTE'
cd /opt/go-edtech
test -s .env || { echo 'Создайте /opt/go-edtech/.env с DATABASE_URL готовой БД'; exit 1; }
python3 -m venv frontend/.venv
frontend/.venv/bin/python -m pip install -r frontend/requirements.txt
(cd frontend/solve-board && npm ci --no-audit --no-fund && npm run build)

cat > /etc/systemd/system/go-edtech.service <<'SERVICE'
[Unit]
Description=Go Edtech
After=network.target

[Service]
WorkingDirectory=/opt/go-edtech
Environment=PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin
Environment=DJANGO_DEBUG=false
Environment=BACKEND_ADDR=0.0.0.0:8080
Environment=BOARD_ORIGIN=http://201.8.94.39:8080
Environment=DJANGO_ALLOWED_HOSTS=201.8.94.39,127.0.0.1,localhost
EnvironmentFile=/opt/go-edtech/.env
ExecStart=/bin/bash /opt/go-edtech/run.sh
Restart=on-failure

[Install]
WantedBy=multi-user.target
SERVICE
systemctl daemon-reload
systemctl enable go-edtech
systemctl restart go-edtech
curl --fail --retry 30 --retry-connrefused --retry-delay 2 --retry-max-time 90 \
    --max-time 5 http://127.0.0.1:8080/login/ > /dev/null
systemctl is-active --quiet go-edtech
REMOTE

echo 'Готово: http://201.8.94.39:8080'
