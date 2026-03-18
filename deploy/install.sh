#!/usr/bin/env bash
set -euo pipefail

APP_NAME="singdns-panel"
INSTALL_DIR="/opt/singdns-panel"
APP_DIR="$INSTALL_DIR/app"
BIN_PATH="$INSTALL_DIR/$APP_NAME"
SERVICE_FILE="/etc/systemd/system/$APP_NAME.service"
SUDOERS_FILE="/etc/sudoers.d/$APP_NAME"
RUN_USER="panel"
PORT="9999"

if [[ $EUID -ne 0 ]]; then
  echo "请用 root 运行安装脚本"
  exit 1
fi

if ! command -v go >/dev/null 2>&1 || ! command -v rsync >/dev/null 2>&1; then
  apt update
  apt install -y golang-go sudo rsync
fi

SRC_DIR="$(cd "$(dirname "$0")/.." && pwd)"
useradd -r -s /usr/sbin/nologin -d "$INSTALL_DIR" "$RUN_USER" 2>/dev/null || true
mkdir -p "$APP_DIR"
rsync -a --delete "$SRC_DIR/" "$APP_DIR/"
mkdir -p "$APP_DIR/logs"

install -m 755 "$APP_DIR/scripts/sbctl.sh" /usr/local/bin/sbctl.sh
install -m 755 "$APP_DIR/scripts/mdctl.sh" /usr/local/bin/mdctl.sh

cd "$APP_DIR"
if [[ ! -f configs/panel.json ]]; then
  go run ./cmd/server init-config configs/panel.json
fi
go mod tidy
go build -o "$BIN_PATH" ./cmd/server

cp "$APP_DIR/deploy/sudoers.singdns-panel" "$SUDOERS_FILE"
chmod 440 "$SUDOERS_FILE"
visudo -c

cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=SingDNS Panel
After=network.target

[Service]
User=$RUN_USER
Group=$RUN_USER
WorkingDirectory=$APP_DIR
Environment=SINGDNS_CONFIG=$APP_DIR/configs/panel.json
ExecStart=$BIN_PATH
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

chown -R "$RUN_USER:$RUN_USER" "$APP_DIR" "$BIN_PATH"
systemctl daemon-reload
systemctl enable --now "$APP_NAME"

echo "安装完成。访问: http://$(hostname -I | awk '{print $1}'):$PORT"
echo "如果你还没改密码，请编辑: $APP_DIR/configs/panel.json"
