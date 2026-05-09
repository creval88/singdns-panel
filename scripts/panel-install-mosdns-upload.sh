#!/usr/bin/env bash
set -euo pipefail

if [[ $EUID -ne 0 ]]; then
  echo "panel-install-mosdns-upload.sh must run as root" >&2
  exit 1
fi

CORE_IN="${1:-}"
CFG_IN="${2:-}"
NAME_IN="${3:-}"

if [[ -z "$CORE_IN" || ! -s "$CORE_IN" ]]; then
  echo "missing mosdns core file" >&2
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y unzip tar ca-certificates rsync || apt-get install -y unzip tar ca-certificates || true

BIN_PATH=/cus/bin
CONFIG_PATH=/cus/mosdns
MOSDNS_BIN=$BIN_PATH/mosdns
SERVICE_FILE=/etc/systemd/system/mosdns.service
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

mkdir -p "$BIN_PATH" "$CONFIG_PATH"

case "$NAME_IN" in
  *.zip)
    unzip -oq "$CORE_IN" -d "$TMP_DIR/core_extract"
    SRC=$(find "$TMP_DIR/core_extract" -type f -name mosdns | head -n1) ;;
  *.tar.gz|*.tgz)
    mkdir -p "$TMP_DIR/core_extract"
    tar -xzf "$CORE_IN" -C "$TMP_DIR/core_extract" || true
    SRC=$(find "$TMP_DIR/core_extract" -type f -name mosdns | head -n1) ;;
  *.tar)
    mkdir -p "$TMP_DIR/core_extract"
    tar -xf "$CORE_IN" -C "$TMP_DIR/core_extract" || true
    SRC=$(find "$TMP_DIR/core_extract" -type f -name mosdns | head -n1) ;;
  *)
    SRC="$CORE_IN" ;;
esac

if [[ -z "$SRC" || ! -s "$SRC" ]]; then
  echo '未在上传包中找到 mosdns 可执行文件' >&2
  exit 1
fi
install -m 755 "$SRC" "$MOSDNS_BIN"

if [[ -n "$CFG_IN" && -s "$CFG_IN" ]]; then
  unzip -oq "$CFG_IN" -d "$TMP_DIR/cfg"
  if command -v rsync >/dev/null 2>&1; then
    rsync -a "$TMP_DIR/cfg"/ "$CONFIG_PATH"/
  else
    cp -a "$TMP_DIR/cfg"/. "$CONFIG_PATH"/
  fi
fi

chmod -R u=rwX,go=rX "$CONFIG_PATH"
cat > "$SERVICE_FILE" <<'UNIT'
[Unit]
Description=MosDNS Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/cus/bin/mosdns start -c /cus/mosdns/config_custom.yaml -d /cus/mosdns
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable mosdns
systemctl restart mosdns || systemctl start mosdns
