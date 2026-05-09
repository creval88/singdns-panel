#!/usr/bin/env bash
set -euo pipefail

if [[ $EUID -ne 0 ]]; then
  echo "panel-enable-ip-forward.sh must run as root" >&2
  exit 1
fi

mkdir -p /etc/sysctl.d
cat > /etc/sysctl.d/99-ipforward.conf <<'EOF'
net.ipv4.ip_forward=1
net.ipv6.conf.all.forwarding=1
EOF

sysctl -w net.ipv4.ip_forward=1
sysctl -w net.ipv6.conf.all.forwarding=1
sysctl --system
