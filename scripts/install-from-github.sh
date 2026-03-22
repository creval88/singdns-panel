#!/usr/bin/env bash
set -euo pipefail

# 一键安装：从 GitHub 拉取最新发布包并执行 install.sh
# 用法（默认 beta 渠道）：
#   curl -fsSL https://raw.githubusercontent.com/creval88/singdns-panel/main/scripts/install-from-github.sh | sudo bash
# 可选环境变量：
#   CHANNEL=stable REPO=creval88/singdns-panel MANIFEST_URL=https://raw.githubusercontent.com/creval88/singdns-panel/main/updates/latest.json

REPO="${REPO:-creval88/singdns-panel}"
CHANNEL="${CHANNEL:-beta}"
ARCH="${ARCH:-}"
MANIFEST_URL="${MANIFEST_URL:-https://raw.githubusercontent.com/${REPO}/main/updates/latest.json}"
WORK_DIR="${WORK_DIR:-/tmp/singdns-panel-install}"
KEEP_WORKDIR="${KEEP_WORKDIR:-0}"

if [[ $EUID -ne 0 ]]; then
  echo "[ERR] 请使用 root 运行（例如: curl ... | sudo bash）"
  exit 1
fi

case "$CHANNEL" in
  beta|stable) ;;
  *)
    echo "[ERR] CHANNEL 仅支持 beta/stable，当前: $CHANNEL"
    exit 1
    ;;
esac

detect_arch() {
  if [[ -n "$ARCH" ]]; then
    echo "$ARCH"
    return
  fi

  local m
  m="$(uname -m)"
  case "$m" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *)
      echo "[ERR] 不支持的 CPU 架构: $m（可手动传 ARCH=amd64 或 ARCH=arm64）" >&2
      exit 1
      ;;
  esac
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1
}

install_deps_if_needed() {
  local missing=()
  need_cmd curl || missing+=(curl)
  need_cmd tar || missing+=(tar)
  need_cmd python3 || missing+=(python3)

  if [[ ${#missing[@]} -eq 0 ]]; then
    return
  fi

  if need_cmd apt-get; then
    echo "[INFO] 安装依赖: ${missing[*]}"
    apt-get update -y
    apt-get install -y ca-certificates "${missing[@]}"
  else
    echo "[ERR] 缺少依赖: ${missing[*]}，且系统无 apt-get，请手动安装后重试"
    exit 1
  fi
}

ARCH="$(detect_arch)"
install_deps_if_needed

mkdir -p "$WORK_DIR"
MANIFEST_PATH="$WORK_DIR/latest.json"
PACKAGE_PATH="$WORK_DIR/singdns-panel.tar.gz"
EXTRACT_DIR="$WORK_DIR/release"

cleanup() {
  if [[ "$KEEP_WORKDIR" == "1" ]]; then
    echo "[INFO] 保留临时目录: $WORK_DIR"
    return
  fi
  rm -rf "$WORK_DIR"
}
trap cleanup EXIT

echo "[1/5] 获取 manifest: $MANIFEST_URL"
curl -fL "$MANIFEST_URL" -o "$MANIFEST_PATH"

echo "[2/5] 解析发布包（channel=$CHANNEL, arch=$ARCH）"
RELEASE_INFO="$(python3 - <<'PY' "$MANIFEST_PATH" "$CHANNEL" "$ARCH"
import json
import sys
from pathlib import Path

p = Path(sys.argv[1])
channel = sys.argv[2]
arch = sys.argv[3]

try:
    data = json.loads(p.read_text(encoding='utf-8'))
except Exception as e:
    print(f'ERR|manifest 解析失败: {e}')
    sys.exit(0)

pkg = None
channels = data.get('channels')
if isinstance(channels, dict):
    ch = channels.get(channel)
    if isinstance(ch, dict):
        item = ch.get(arch)
        if isinstance(item, dict):
            pkg = item

if pkg is None:
    ch = data.get(channel)
    if isinstance(ch, dict):
        item = ch.get(arch)
        if isinstance(item, dict):
            pkg = item

if pkg is None:
    print('ERR|manifest 未找到对应 channel/arch 的发布包')
    sys.exit(0)

url = str(pkg.get('url', '')).strip()
sha = str(pkg.get('sha256', '')).strip().lower()
ver = str(pkg.get('version', '')).strip()

if not url:
    print('ERR|manifest 中 url 为空')
    sys.exit(0)

print(f'OK|{url}|{sha}|{ver}')
PY
)"

if [[ "$RELEASE_INFO" == ERR* ]]; then
  echo "[ERR] ${RELEASE_INFO#ERR|}"
  exit 1
fi

IFS='|' read -r _ PACKAGE_URL PACKAGE_SHA PACKAGE_VERSION <<<"$RELEASE_INFO"

echo "[INFO] 命中版本: ${PACKAGE_VERSION:-unknown}"
echo "[INFO] 下载地址: $PACKAGE_URL"

echo "[3/5] 下载发布包"
curl -fL "$PACKAGE_URL" -o "$PACKAGE_PATH"

if [[ -n "$PACKAGE_SHA" ]]; then
  echo "[INFO] 校验 sha256"
  DOWNLOADED_SHA="$(sha256sum "$PACKAGE_PATH" | awk '{print tolower($1)}')"
  if [[ "$DOWNLOADED_SHA" != "$PACKAGE_SHA" ]]; then
    echo "[ERR] sha256 校验失败"
    echo "      expected: $PACKAGE_SHA"
    echo "      got     : $DOWNLOADED_SHA"
    exit 1
  fi
fi

echo "[4/5] 解压发布包"
rm -rf "$EXTRACT_DIR"
mkdir -p "$EXTRACT_DIR"
tar xzf "$PACKAGE_PATH" -C "$EXTRACT_DIR"

REL_DIR="$EXTRACT_DIR/singdns-panel-release"
if [[ ! -x "$REL_DIR/install.sh" ]]; then
  echo "[ERR] 发布包缺少 install.sh: $REL_DIR/install.sh"
  exit 1
fi

echo "[5/5] 执行安装脚本"
cd "$REL_DIR"
bash install.sh

echo "[DONE] 安装完成"
