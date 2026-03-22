#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="$ROOT_DIR/dist"
REL_DIR="$OUT_DIR/singdns-panel-release"
VERSION="${1:-$(date +%Y%m%d-%H%M%S)}"
ARCH_RAW="${RELEASE_ARCH:-$(uname -m)}"

normalize_arch() {
  case "$1" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *) echo "$1" ;;
  esac
}

ARCH="$(normalize_arch "$ARCH_RAW")"

mkdir -p "$OUT_DIR"
rm -rf "$REL_DIR"
mkdir -p "$REL_DIR"

cd "$ROOT_DIR"

echo "[0/5] 发布前敏感信息自检..."
if find . -path './.git' -prune -o -path './dist' -prune -o -type f \( -name 'url.txt' -o -name 'panel.json' \) -print | grep -v '^./configs/panel.example.json$' | grep -q .; then
  echo "错误: 检测到潜在运行时敏感文件，请勿打包真实配置/url.txt"
  find . -path './.git' -prune -o -path './dist' -prune -o -type f \( -name 'url.txt' -o -name 'panel.json' \) -print | grep -v '^./configs/panel.example.json$' || true
  exit 1
fi
if rg -n --hidden --glob '!dist/**' --glob '!.git/**' '(https?://).*(token|subscribe|subscription|sub=|auth=)' . >/tmp/singdns-release-sensitive-check.txt 2>/dev/null; then
  echo "错误: 检测到疑似敏感订阅/鉴权链接，请先清理后再打包"
  cat /tmp/singdns-release-sensitive-check.txt
  exit 1
fi

echo "[1/5] 准备配置模板..."
cp configs/panel.example.json "$REL_DIR/panel.json"

echo "[2/5] 复制脚本与部署文件..."
cp scripts/sbctl.sh "$REL_DIR/sbctl.sh"
cp scripts/mdctl.sh "$REL_DIR/mdctl.sh"
cp deploy/sudoers.singdns-panel "$REL_DIR/sudoers.singdns-panel"
printf '%s\n' "$VERSION" > "$REL_DIR/VERSION"

cat > "$REL_DIR/singdns-panel.service" <<'EOF'
[Unit]
Description=SingDNS Panel
After=network.target

[Service]
User=panel
Group=panel
WorkingDirectory=/opt/singdns-panel/app
Environment=SINGDNS_CONFIG=/opt/singdns-panel/app/configs/panel.json
ExecStart=/opt/singdns-panel/singdns-panel
Restart=always
RestartSec=3
KillMode=process

[Install]
WantedBy=multi-user.target
EOF

echo "[3/5] 编译二进制..."
mkdir -p "$REL_DIR/bin"
if command -v go >/dev/null 2>&1; then
  go mod tidy
  GOOS=linux GOARCH="$ARCH" CGO_ENABLED=0 go build -ldflags "-X main.Version=${VERSION}" -o "$REL_DIR/bin/singdns-panel" ./cmd/server
else
  echo "警告: 当前环境没有 go，跳过编译。你需要在目标机自行编译。"
fi

echo "[4/5] 生成安装脚本..."
cat > "$REL_DIR/install.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
APP_NAME="singdns-panel"
BASE_DIR="/opt/singdns-panel"
APP_DIR="$BASE_DIR/app"
BIN_PATH="$BASE_DIR/$APP_NAME"
SERVICE_FILE="/etc/systemd/system/$APP_NAME.service"
SUDOERS_FILE="/etc/sudoers.d/$APP_NAME"
RUN_USER="panel"

if [[ $EUID -ne 0 ]]; then
  echo "请用 root 运行 install.sh"
  exit 1
fi

mkdir -p "$APP_DIR/configs" "$APP_DIR/logs" "$BASE_DIR/updates"
useradd -r -s /usr/sbin/nologin -d "$BASE_DIR" "$RUN_USER" 2>/dev/null || true

install -m 755 sbctl.sh /usr/local/bin/sbctl.sh
install -m 755 mdctl.sh /usr/local/bin/mdctl.sh
mkdir -p /etc/sudoers.d
cp sudoers.singdns-panel "$SUDOERS_FILE"
chmod 440 "$SUDOERS_FILE"
visudo -c

cp -n panel.json "$APP_DIR/configs/panel.json"
cp singdns-panel.service "$SERVICE_FILE"
if [[ -f "$APP_DIR/configs/panel.json" ]]; then
  chown "$RUN_USER:$RUN_USER" "$APP_DIR/configs/panel.json" 2>/dev/null || true
  chmod 640 "$APP_DIR/configs/panel.json" 2>/dev/null || true
fi

if [[ -f bin/singdns-panel ]]; then
  install -m 755 bin/singdns-panel "$BIN_PATH"
else
  echo "缺少预编译二进制 bin/singdns-panel，安装失败"
  exit 1
fi

# 订阅日志文件（统一落盘到 /etc/sing-box，避免页面无记录）
mkdir -p /etc/sing-box
install -o "$RUN_USER" -g "$RUN_USER" -m 664 /dev/null /etc/sing-box/subscription-history.log
install -o "$RUN_USER" -g "$RUN_USER" -m 664 /dev/null /etc/sing-box/subscription-updates.log

chown -R "$RUN_USER:$RUN_USER" "$APP_DIR" "$BASE_DIR/updates" "$BIN_PATH"
chmod 640 "$APP_DIR/configs/panel.json" 2>/dev/null || true
chmod 775 "$BASE_DIR/updates" 2>/dev/null || true

systemctl daemon-reload
systemctl enable --now "$APP_NAME"

echo "安装完成: http://$(hostname -I | awk '{print $1}'):9999"
EOF
chmod +x "$REL_DIR/install.sh"

cat > "$REL_DIR/upgrade.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
APP_NAME="singdns-panel"
BASE_DIR="/opt/singdns-panel"
APP_DIR="$BASE_DIR/app"
BIN_PATH="$BASE_DIR/$APP_NAME"
SERVICE_FILE="/etc/systemd/system/$APP_NAME.service"
SUDOERS_FILE="/etc/sudoers.d/$APP_NAME"
RUN_USER="panel"

cd "$(dirname "$0")"

if [[ $EUID -ne 0 ]]; then
  echo "请用 root 运行 upgrade.sh"
  exit 1
fi

BACKUP_ROOT="$BASE_DIR/backups/panel-upgrade"
TS="$(date +%Y%m%d-%H%M%S)"
BACKUP_DIR="$BACKUP_ROOT/$TS"
mkdir -p "$BACKUP_DIR"

# 备份关键文件（用于失败回滚）
cp -a "$BIN_PATH" "$BACKUP_DIR/singdns-panel.bak" 2>/dev/null || true
cp -a "$SERVICE_FILE" "$BACKUP_DIR/singdns-panel.service.bak" 2>/dev/null || true
cp -a "$SUDOERS_FILE" "$BACKUP_DIR/sudoers.singdns-panel.bak" 2>/dev/null || true

rollback() {
  echo "[rollback] 检测到升级失败，开始回滚..."
  if [[ -f "$BACKUP_DIR/singdns-panel.bak" ]]; then
    install -m 755 "$BACKUP_DIR/singdns-panel.bak" "$BIN_PATH" || true
  fi
  if [[ -f "$BACKUP_DIR/singdns-panel.service.bak" ]]; then
    cp "$BACKUP_DIR/singdns-panel.service.bak" "$SERVICE_FILE" || true
  fi
  if [[ -f "$BACKUP_DIR/sudoers.singdns-panel.bak" ]]; then
    cp "$BACKUP_DIR/sudoers.singdns-panel.bak" "$SUDOERS_FILE" || true
    chmod 440 "$SUDOERS_FILE" || true
    visudo -c || true
  fi
  systemctl daemon-reload || true
  systemctl start "$APP_NAME" || true
  systemctl is-active --quiet "$APP_NAME" || true
  echo "[rollback] 已尝试恢复上一个可用版本"
}
trap 'rollback' ERR

install -m 755 sbctl.sh /usr/local/bin/sbctl.sh
install -m 755 mdctl.sh /usr/local/bin/mdctl.sh
mkdir -p /etc/sudoers.d
cp sudoers.singdns-panel "$SUDOERS_FILE"
chmod 440 "$SUDOERS_FILE"
visudo -c
cp singdns-panel.service "$SERVICE_FILE"
mkdir -p "$BASE_DIR/updates"

if [[ -f bin/singdns-panel ]]; then
  install -m 755 bin/singdns-panel "$BIN_PATH"
else
  echo "缺少预编译二进制，升级失败"
  exit 1
fi

# 修正权限（避免 panel.json 被 root 覆盖后面板无法保存配置）
mkdir -p "$APP_DIR/configs" "$APP_DIR/logs" "$BASE_DIR/updates"
chown -R "$RUN_USER:$RUN_USER" "$APP_DIR" "$BASE_DIR/updates" "$BIN_PATH" 2>/dev/null || true
chmod 775 "$BASE_DIR/updates" 2>/dev/null || true
chmod 750 "$APP_DIR/configs" 2>/dev/null || true
chmod 640 "$APP_DIR/configs/panel.json" 2>/dev/null || true

# 订阅日志文件兜底（统一落盘到 /etc/sing-box）
mkdir -p /etc/sing-box
install -o "$RUN_USER" -g "$RUN_USER" -m 664 /dev/null /etc/sing-box/subscription-history.log
install -o "$RUN_USER" -g "$RUN_USER" -m 664 /dev/null /etc/sing-box/subscription-updates.log

systemctl daemon-reload

# 平滑升级：优先 systemd 标准停启，避免 -9 强杀
if systemctl is-active --quiet "$APP_NAME"; then
  systemctl stop "$APP_NAME"
fi

systemctl start "$APP_NAME"
systemctl is-active --quiet "$APP_NAME" || { echo "升级后服务未启动"; exit 1; }

# 成功后取消错误回滚陷阱
trap - ERR

chown -R "$RUN_USER:$RUN_USER" "$BASE_DIR/updates" 2>/dev/null || true
chmod 775 "$BASE_DIR/updates" 2>/dev/null || true

echo "升级完成（已备份到: $BACKUP_DIR）"
EOF
chmod +x "$REL_DIR/upgrade.sh"

cat > "$REL_DIR/uninstall.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
APP_NAME="singdns-panel"
BASE_DIR="/opt/singdns-panel"
SERVICE_FILE="/etc/systemd/system/$APP_NAME.service"
SUDOERS_FILE="/etc/sudoers.d/$APP_NAME"

if [[ $EUID -ne 0 ]]; then
  echo "请用 root 运行 uninstall.sh"
  exit 1
fi

read -r -p "是否卸载 SingDNS Panel？默认保留 configs/logs [y/N]: " confirm
[[ "$confirm" =~ ^[Yy]$ ]] || exit 0

systemctl stop "$APP_NAME" 2>/dev/null || true
systemctl disable "$APP_NAME" 2>/dev/null || true
rm -f "$SERVICE_FILE" "$SUDOERS_FILE" /opt/singdns-panel/singdns-panel
systemctl daemon-reload

echo "已卸载主程序。配置与日志仍保留在 /opt/singdns-panel/app"
EOF
chmod +x "$REL_DIR/uninstall.sh"

echo "[5/5] 打包..."
TAR_NAME="singdns-panel-${VERSION}-${ARCH}.tar.gz"
cd "$OUT_DIR"
tar czf "$TAR_NAME" "$(basename "$REL_DIR")"
echo "完成: $OUT_DIR/$TAR_NAME"
