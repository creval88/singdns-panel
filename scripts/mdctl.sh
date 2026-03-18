#!/bin/bash
set -euo pipefail
SERVICE_NAME="mosdns"
cmd="${1:-}"
case "$cmd" in
  status) systemctl status "$SERVICE_NAME" --no-pager ;;
  start|stop|restart) systemctl "$cmd" "$SERVICE_NAME" ;;
  logs) journalctl -u "$SERVICE_NAME" -n 100 --no-pager ;;
  *) echo "usage: $0 {status|start|stop|restart|logs}"; exit 1 ;;
esac
