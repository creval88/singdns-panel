#!/bin/bash
#
# 远程: 安装 singdns-panel (在 Debian 上执行)
# 默认配置:
#   端口: 9999
#   用户名: admin
#   密码: admin
#
# 自定义用法:
#   PORT=8888 PASSWORD=mypassword ./install-singdns.sh
#

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

PANEL_PORT="${PORT:-9999}"
ADMIN_PASSWORD="${PASSWORD:-admin}"

echo -e "${GREEN}=== SingDNS Panel 安装 ===${NC}"
echo "端口: $PANEL_PORT"
echo "密码: $ADMIN_PASSWORD"
echo ""

# 检查 root
if [[ $EUID -ne 0 ]]; then
   echo -e "${RED}错误: 请用 root 用户运行${NC}"
   exit 1
fi

# 检查二进制
if [[ ! -f /tmp/singdns-panel ]]; then
    echo -e "${RED}错误: 找不到 /tmp/singdns-panel${NC}"
    echo "请先运行本地脚本上传二进制"
    exit 1
fi

echo -e "${YELLOW}[1/5] 创建用户和目录...${NC}"
useradd -r -s /usr/sbin/nologin -d /opt/singdns-panel panel 2>/dev/null || true
mkdir -p /opt/singdns-panel/{app/configs,app/logs,updates}
chown -R panel:panel /opt/singdns-panel

echo -e "${YELLOW}[2/5] 安装二进制...${NC}"
cp /tmp/singdns-panel /opt/singdns-panel/singdns-panel
chmod +x /opt/singdns-panel/singdns-panel
chown panel:panel /opt/singdns-panel/singdns-panel

echo -e "${YELLOW}[3/5] 生成配置...${NC}"
SESSION_KEY=$(openssl rand -base64 32 2>/dev/null)
PASSWORD_HASH=$(/opt/singdns-panel/singdns-panel hash-password "$ADMIN_PASSWORD" 2>/dev/null | tail -1)

cat > /opt/singdns-panel/app/configs/panel.json << EOF
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
EOF

chown panel:panel /opt/singdns-panel/app/configs/panel.json

echo -e "${YELLOW}[4/5] 配置 systemd 服务...${NC}"
mkdir -p /var/log/singdns-panel
chown panel:panel /var/log/singdns-panel

cat > /etc/systemd/system/singdns-panel.service << 'EOF'
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
EOF

systemctl daemon-reload
systemctl enable --now singdns-panel

# 清理临时文件
rm -f /tmp/singdns-panel /tmp/install-singdns.sh

echo -e "${YELLOW}[5/5] 检查服务状态...${NC}"
systemctl status singdns-panel --no-pager || true

IP_ADDR=$(hostname -I 2>/dev/null | awk '{print $1}')

echo ""
echo -e "${GREEN}=== 安装完成! ===${NC}"
echo -e "访问地址: ${GREEN}http://$IP_ADDR:$PANEL_PORT${NC}"
echo -e "用户名: ${GREEN}admin${NC}"
echo -e "密码: ${GREEN}$ADMIN_PASSWORD${NC}"
echo ""
echo "常用命令:"
echo "  查看日志: journalctl -u singdns-panel -f"
echo "  重启服务: systemctl restart singdns-panel"
echo "  停止服务: systemctl stop singdns-panel"
