#!/usr/bin/env bash
set -euo pipefail

if [[ $EUID -ne 0 ]]; then
  echo "panel-install-mosdns.sh must run as root" >&2
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive

apt-get update
apt-get install -y wget unzip curl lsof tar ca-certificates rsync || apt-get install -y wget unzip curl lsof tar ca-certificates

BIN_PATH=/cus/bin
CONFIG_PATH=/cus/mosdns
MOSDNS_BIN=$BIN_PATH/mosdns
SERVICE_FILE=/etc/systemd/system/mosdns.service
CONFIG_BASE_URL=https://raw.githubusercontent.com/yyysuo/firetv/refs/heads/master/mosdnsconfigupdate/mosdns1225all.zip
CONFIG_UPDATE_URL=https://raw.githubusercontent.com/yyysuo/firetv/refs/heads/master/mosdnsconfigupdate/mosdns20251225allup.zip
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

ARCH_RAW=$(uname -m)
case "$ARCH_RAW" in
  x86_64) ARCH_STR=amd64 ;;
  aarch64) ARCH_STR=arm64 ;;
  *) echo "unsupported arch: $ARCH_RAW" >&2; exit 1 ;;
esac

if command -v ss >/dev/null 2>&1 && ss -lntup 2>/dev/null | grep -q ':53 '; then
  if systemctl is-active --quiet systemd-resolved 2>/dev/null || systemctl is-enabled --quiet systemd-resolved 2>/dev/null; then
    systemctl stop systemd-resolved || true
    systemctl disable systemd-resolved || true
    printf 'nameserver 223.5.5.5\n' > /etc/resolv.conf
  else
    lsof -i :53 || true
  fi
fi

TAG_VERSION=$(curl -fsSL https://api.github.com/repos/yyysuo/mosdns/releases/latest | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)
if [[ -z "$TAG_VERSION" ]]; then
  echo 'failed to resolve latest mosdns release tag' >&2
  exit 1
fi
DOWNLOAD_URL="https://github.com/yyysuo/mosdns/releases/download/${TAG_VERSION}/mosdns-linux-${ARCH_STR}.zip"

systemctl stop mosdns || true
mkdir -p "$BIN_PATH" "$CONFIG_PATH"

curl -fL "$DOWNLOAD_URL" -o "$TMP_DIR/mosdns.zip"
unzip -oq "$TMP_DIR/mosdns.zip" -d "$TMP_DIR/mosdns_extract"
MOSDNS_SRC=$(find "$TMP_DIR/mosdns_extract" -type f -name mosdns | head -n1)
if [[ -z "$MOSDNS_SRC" ]]; then
  echo 'mosdns binary not found in release zip' >&2
  exit 1
fi
install -m 755 "$MOSDNS_SRC" "$MOSDNS_BIN"

curl -fL "$CONFIG_BASE_URL" -o "$TMP_DIR/mosdns_base.zip"
unzip -oq "$TMP_DIR/mosdns_base.zip" -d "$TMP_DIR/mosdns_base"
if command -v rsync >/dev/null 2>&1; then
  rsync -a "$TMP_DIR/mosdns_base"/ "$CONFIG_PATH"/
else
  cp -a "$TMP_DIR/mosdns_base"/. "$CONFIG_PATH"/
fi

if curl -fsSL "$CONFIG_UPDATE_URL" -o "$TMP_DIR/mosdns_update.zip"; then
  unzip -oq "$TMP_DIR/mosdns_update.zip" -d "$TMP_DIR/mosdns_update"
  if command -v rsync >/dev/null 2>&1; then
    rsync -a "$TMP_DIR/mosdns_update"/ "$CONFIG_PATH"/
  else
    cp -a "$TMP_DIR/mosdns_update"/. "$CONFIG_PATH"/
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
