#!/bin/bash
#
# 本地: 上传 singdns-panel 到远程服务器
# 用法: ./upload-binary.sh <目标服务器> [选项]
#
# 选项:
#   -k, --ssh-key PATH     SSH 密钥路径
#   -p, --port PORT        SSH 端口 (默认: 22)
#

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

SSH_USER="root"
TARGET_HOST=""
SSH_KEY=""
SSH_PORT="22"

while [[ $# -gt 0 ]]; do
    case $1 in
        -k|--ssh-key)
            SSH_KEY="$2"
            shift 2
            ;;
        -p|--port)
            SSH_PORT="$2"
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

[[ "$TARGET_HOST" == *"@"* ]] && SSH_USER="${TARGET_HOST%%@*}" && TARGET_HOST="${TARGET_HOST#*@}"

SCP_CMD="scp -P $SSH_PORT -o StrictHostKeyChecking=no"
[[ -n "$SSH_KEY" ]] && SCP_CMD="$SCP_CMD -i $SSH_KEY"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BINARY_PATH="$SCRIPT_DIR/../dist/singdns-panel"
INSTALL_SCRIPT="$SCRIPT_DIR/install-remote.sh"

if [[ ! -f "$BINARY_PATH" ]]; then
    echo -e "${RED}错误: 找不到二进制文件${NC}"
    exit 1
fi

if [[ ! -f "$INSTALL_SCRIPT" ]]; then
    echo -e "${RED}错误: 找不到安装脚本 $INSTALL_SCRIPT${NC}"
    exit 1
fi

echo -e "${GREEN}=== 上传 singdns-panel ===${NC}"
echo "目标: $SSH_USER@$TARGET_HOST"
echo ""

echo -e "${YELLOW}[1/2] 上传二进制和安装脚本...${NC}"
$SCP_CMD "$BINARY_PATH" "$SSH_USER@$TARGET_HOST:/tmp/singdns-panel"
$SCP_CMD "$INSTALL_SCRIPT" "$SSH_USER@$TARGET_HOST:/tmp/install-singdns.sh"

echo -e "${YELLOW}[2/2] 执行安装...${NC}"
ssh -p $SSH_PORT -o StrictHostKeyChecking=no \
    $([[ -n "$SSH_KEY" ]] && echo "-i $SSH_KEY") \
    "$SSH_USER@$TARGET_HOST" \
    "chmod +x /tmp/install-singdns.sh && /tmp/install-singdns.sh"

echo -e "${GREEN}完成!${NC}"
