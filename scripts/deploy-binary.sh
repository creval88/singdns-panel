#!/bin/bash
#
# singdns-panel 轻量部署脚本 (本地已编译好)
# 用法: ./deploy-binary.sh <目标服务器> [SSH用户] [选项]
#
# 选项:
#   -k, --ssh-key PATH     SSH 密钥路径
#   -p, --port PORT        面板端口 (默认: 9999)
#   -w, --password PASS    管理密码 (默认: admin)
#
# 示例:
#   ./deploy-binary.sh 192.168.1.100
#   ./deploy-binary.sh 192.168.1.100 -k ~/.ssh/id_rsa -w mypassword
#

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

SSH_USER="root"
TARGET_HOST=""
SSH_KEY=""
PANEL_PORT="9999"
ADMIN_PASSWORD="admin"

while [[ $# -gt 0 ]]; do
    case $1 in
        -k|--ssh-key)
            SSH_KEY="$2"
            shift 2
            ;;
        -p|--port)
            PANEL_PORT="$2"
            shift 2
            ;;
        -w|--password)
            ADMIN_PASSWORD="$2"
            shift 2
            ;;
        -h|--help)
            echo "用法: $0 <目标服务器> [选项]"
            exit 0
            ;;
        *)
            TARGET_HOST="$1"
            shift
            ;;
    esac
done

if [[ -z "$TARGET_HOST" ]]; then
    echo -e "${RED}用法: $0 <目标服务器> [选项]${NC}"
    exit 1
fi

SSH_CMD="ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
[[ -n "$SSH_KEY" ]] && SSH_CMD="$SSH_CMD -i $SSH_KEY"

SCP_CMD="scp -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
[[ -n "$SSH_KEY" ]] && SCP_CMD="$SCP_CMD -i $SSH_KEY"

[[ "$TARGET_HOST" == *"@"* ]] && SSH_USER="${TARGET_HOST%%@*}" && TARGET_HOST="${TARGET_HOST#*@}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BINARY_PATH="$SCRIPT_DIR/../dist/singdns-panel"

echo -e "${GREEN}=== SingDNS Panel 轻量部署 ===${NC}"
echo "目标: $SSH_USER@$TARGET_HOST"
echo ""

# 检查二进制
if [[ ! -f "$BINARY_PATH" ]]; then
    echo -e "${RED}错误: 找不到二进制文件 $BINARY_PATH${NC}"
    echo "请先运行: cd ../ && go build -o dist/singdns-panel ./cmd/server"
    exit 1
fi

echo -e "${YELLOW}[1/5] 检查远程服务器...${NC}"
$SSH_CMD "$SSH_USER@$TARGET_HOST" "echo 'OK'" || exit 1

echo -e "${YELLOW}[2/5] 创建目录...${NC}"
$SSH_CMD "$SSH_USER@$TARGET_HOST" << 'EOF'
mkdir -p /opt/singdns-panel/{app/configs,app/logs,updates}
useradd -r -s /usr/sbin/nologin -d /opt/singdns-panel panel || true
chown -R panel:panel /opt/singdns-panel
EOF

echo -e "${YELLOW}[3/5] 上传二进制和配置...${NC}"
$SCP_CMD "$BINARY_PATH" "$SSH_USER@$TARGET_HOST:/opt/singdns-panel/singdns-panel"
$SSH_CMD "$SSH_USER@$TARGET_HOST" "chmod +x /opt/singdns-panel/singdns-panel && chown panel:panel /opt/singdns-panel/singdns-panel"

echo -e "${YELLOW}[4/5] 生成配置...${NC}"
$SSH_CMD "$SSH_USER@$TARGET_HOST" << EOF
SESSION_KEY=\$(openssl rand -base64 32)
PASSWORD_HASH=\$(/opt/singdns-panel/singdns-panel hash-password '$ADMIN_PASSWORD' 2>/dev/null | tail -1)

cat > /opt/singdns-panel/app/configs/panel.json << PANELEOF
{
  "listen": ":$PANEL_PORT",
  "session_key": "\$SESSION_KEY",
  "audit_log": "logs/audit.log",
  "auth": {
    "username": "admin",
    "password_hash": "\$PASSWORD_HASH"
  },
  "panel_update": {
    "release_dir": "/opt/singdns-panel/updates",
    "upgrade_command": "",
    "base_url": "https://github.com/creval88/singdns-panel/releases/latest/download/latest.json",
    "channel": "beta",
    "arch": "amd64"
  },
  "services": {
    "singbox": {
      "service_name": "sing-box",
      "config_path": "/etc/sing-box/config.json",
      "url_path": "/etc/sing-box/url.txt",
      "bin_path": "/usr/local/bin/sing-box",
      "ctl_path": "/usr/local/bin/sbctl.sh"
    },
    "mosdns": {
      "service_name": "mosdns",
      "ctl_path": "/usr/local/bin/mdctl.sh",
      "web_url": "http://10.0.0.8:9099/log"
    }
  }
}
PANELEOF
chown panel:panel /opt/singdns-panel/app/configs/panel.json
EOF

echo -e "${YELLOW}[5/5] 配置 systemd 并启动...${NC}"
$SSH_CMD "$SSH_USER@$TARGET_HOST" << 'EOF'
mkdir -p /var/log/singdns-panel
chown panel:panel /var/log/singdns-panel

cat > /etc/systemd/system/singdns-panel.service << 'SERVICEEOF'
[Unit]
Description=SingDNS Panel
After=network.target

[Service]
Type=simple
User=panel
Group=panel
WorkingDirectory=/opt/singdns-panel/app
ExecStart=/opt/singdns-panel/singdns-panel -config /opt/singdns-panel/app/configs/panel.json
Restart=always
RestartSec=5
StandardOutput=append:/var/log/singdns-panel/stdout.log
StandardError=append:/var/log/singdns-panel/stderr.log

[Install]
WantedBy=multi-user.target
SERVICEEOF

systemctl daemon-reload
systemctl enable --now singdns-panel
EOF

$SSH_CMD "$SSH_USER@$TARGET_HOST" "systemctl status singdns-panel --no-pager || true"

echo ""
echo -e "${GREEN}=== 部署完成! ===${NC}"
echo -e "访问: ${GREEN}http://$TARGET_HOST:$PANEL_PORT${NC}"
echo -e "用户: ${GREEN}admin${NC}"
echo -e "密码: ${GREEN}$ADMIN_PASSWORD${NC}"
