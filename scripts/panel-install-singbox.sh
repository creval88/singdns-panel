#!/usr/bin/env bash
set -euo pipefail

ENABLE_IP_FORWARD="${1:-${ENABLE_IP_FORWARD:-1}}"

if [[ $EUID -ne 0 ]]; then
  echo "panel-install-singbox.sh must run as root" >&2
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive

if ! command -v curl >/dev/null 2>&1; then
  apt-get update
  apt-get install -y curl ca-certificates
fi

curl -fsSL https://sing-box.app/install.sh | bash
systemctl enable sing-box
systemctl restart sing-box || systemctl start sing-box

if [[ "$ENABLE_IP_FORWARD" == "1" || "$ENABLE_IP_FORWARD" == "true" ]]; then
  /usr/local/bin/singdns-panel-enable-ip-forward.sh
fi
