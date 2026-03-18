#!/bin/bash
#
# singdns-panel 自动部署脚本
# 用法: ./deploy-remote.sh <目标服务器> [选项]
#
# 选项:
#   -k, --ssh-key PATH     SSH 密钥路径
#   -p, --port PORT        面板端口 (默认: 9999)
#   -w, --password PASS    管理密码 (默认: admin)
#
# 示例:
#   ./deploy-remote.sh 192.168.1.100
#   ./deploy-remote.sh 192.168.1.100 -k ~/.ssh/id_rsa -w mypassword
#

set -e

# 颜色
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# 默认值
SSH_USER="root"
TARGET_HOST=""
SSH_KEY=""
PANEL_PORT="9999"
ADMIN_PASSWORD="admin"

# 解析参数
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
            echo "选项:"
            echo "  -k, --ssh-key PATH   SSH 密钥路径"
            echo "  -p, --port PORT      面板端口 (默认: 9999)"
            echo "  -w, --password PASS  管理密码 (默认: admin)"
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
    echo "查看帮助: $0 --help"
    exit 1
fi

# 构建 SSH 命令
SSH_CMD="ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
if [[ -n "$SSH_KEY" ]]; then
    SSH_CMD="$SSH_CMD -i $SSH_KEY"
fi

SCP_CMD="scp -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
if [[ -n "$SSH_KEY" ]]; then
    SCP_CMD="$SCP_CMD -i $SSH_KEY"
fi

# 获取目标用户
if [[ "$TARGET_HOST" == *"@"* ]]; then
    SSH_USER="${TARGET_HOST%%@*}"
    TARGET_HOST="${TARGET_HOST#*@}"
fi

echo -e "${GREEN}=== SingDNS Panel 自动部署 ===${NC}"
echo "目标服务器: $SSH_USER@$TARGET_HOST"
echo "面板端口: $PANEL_PORT"
echo ""

# 检查本地项目
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

if [[ ! -d "$PROJECT_DIR/singdns-panel" ]]; then
    echo -e "${RED}错误: 找不到 singdns-panel 项目目录${NC}"
    exit 1
fi

# === 执行部署 ===

echo -e "${YELLOW}[1/7] 检查远程服务器...${NC}"
$SSH_CMD "$SSH_USER@$TARGET_HOST" "echo 'SSH 连接成功'" || {
    echo -e "${RED}错误: 无法连接到远程服务器${NC}"
    exit 1
}

echo -e "${YELLOW}[2/7] 安装依赖 (Go, git)...${NC}"
$SSH_CMD "$SSH_USER@$TARGET_HOST" "apt update && apt install -y golang-go sudo git openssl" || {
    echo -e "${RED}错误: 安装依赖失败${NC}"
    exit 1
}

echo -e "${YELLOW}[3/7] 创建用户和目录...${NC}"
$SSH_CMD "$SSH_USER@$TARGET_HOST" << 'EOF'
useradd -r -s /usr/sbin/nologin -d /opt/singdns-panel panel || true
mkdir -p /opt/singdns-panel
chown -R panel:panel /opt/singdns-panel
EOF

echo -e "${YELLOW}[4/7] 上传项目文件...${NC}"
$SCP_CMD -r "$PROJECT_DIR/singdns-panel/"* "$SSH_USER@$TARGET_HOST:/opt/singdns-panel/app/" 2>/dev/null

echo -e "${YELLOW}[5/7] 编译项目并生成密码...${NC}"
$SSH_CMD "$SSH_USER@$TARGET_HOST" << 'EOF'
cd /opt/singdns-panel/app
mkdir -p logs configs
go mod tidy
go build -o /opt/singdns-panel/singdns-panel ./cmd/server
chown panel:panel /opt/singdns-panel/singdns-panel
EOF

PASSWORD_HASH=$($SSH_CMD "$SSH_USER@$TARGET_HOST" "cd /opt/singdns-panel/app && go run ./cmd/server hash-password '$ADMIN_PASSWORD'" 2>/dev/null | tail -1)

SESSION_KEY=$($SSH_CMD "$SSH_USER@$TARGET_HOST" "openssl rand -base64 32" 2>/dev/null)

echo -e "${YELLOW}[6/7] 配置 systemd 服务...${NC}"
$SSH_CMD "$SSH_USER@$TARGET_HOST" << EOF
# 生成配置
cat > /opt/singdns-panel/app/configs/panel.json << PANELEOF
{
  "listen": ":$PANEL_PORT",
  "session_key": "$SESSION_KEY",
  "audit_log": "logs/audit.log",
  "auth": {
    "username": "admin",
    "password_hash": "$PASSWORD_HASH"
  },
  "panel_update": {
    "release_dir": "/opt/singdns-panel/updates",
    "upgrade_command": ""
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

# 创建日志目录
mkdir -p /var/log/singdns-panel
chown panel:panel /var/log/singdns-panel

# 安装 systemd 服务
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

echo -e "${YELLOW}[7/7] 验证服务状态...${NC}"
$SSH_CMD "$SSH_USER@$TARGET_HOST" "systemctl status singdns-panel --no-pager || true"

echo ""
echo -e "${GREEN}=== 部署完成! ===${NC}"
echo -e "访问地址: ${GREEN}http://$TARGET_HOST:$PANEL_PORT${NC}"
echo -e "用户名: ${GREEN}admin${NC}"
echo -e "密码: ${GREEN}$ADMIN_PASSWORD${NC}"
echo ""
echo -e "${YELLOW}后续命令:${NC}"
echo "  查看日志: ssh $SSH_USER@$TARGET_HOST 'tail -f /var/log/singdns-panel/stdout.log'"
echo "  重启服务: ssh $SSH_USER@$TARGET_HOST 'systemctl restart singdns-panel'"
echo "  停止服务: ssh $SSH_USER@$TARGET_HOST 'systemctl stop singdns-panel'"
