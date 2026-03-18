#!/bin/bash
set -euo pipefail
BIN_PATH="/usr/local/bin/sing-box"
CONF_DIR="/etc/sing-box"
CONF_FILE="${CONF_DIR}/config.json"
URL_FILE="${CONF_DIR}/url.txt"
SERVICE_NAME="sing-box"
SCRIPT_PATH=$(readlink -f "$0")
TMP_FILE="/tmp/singbox-web-check.json"

cron_line() {
  crontab -l 2>/dev/null | grep "$SCRIPT_PATH update" || true
}

cron_summary() {
  local line
  line="$(cron_line)"
  if [[ -z "$line" ]]; then
    echo "disabled"
    return
  fi
  local c_hour c_day step
  c_hour=$(echo "$line" | awk '{print $2}')
  c_day=$(echo "$line" | awk '{print $3}')
  if [[ "$c_day" == "*" ]]; then
    echo "每天 ${c_hour}:00"
  elif [[ "$c_day" == */* ]]; then
    step=$(echo "$c_day" | cut -d'/' -f2)
    echo "每隔 ${step} 天 ${c_hour}:00"
  else
    echo "$line"
  fi
}

safe_update() {
  local download_url
  download_url=$(cat "$URL_FILE")
  cp "$CONF_FILE" "${CONF_FILE}.bak" 2>/dev/null || true
  wget -O "$TMP_FILE" "$download_url"
  "$BIN_PATH" check -c "$TMP_FILE"
  mv "$TMP_FILE" "$CONF_FILE"
  systemctl restart "$SERVICE_NAME"
}

save_config() {
  local src="${2:?missing temp file}"
  cp "$CONF_FILE" "${CONF_FILE}.web.bak" 2>/dev/null || true
  "$BIN_PATH" check -c "$src"
  cp "$src" "$CONF_FILE"
}

write_file() {
  local src="${2:?missing source file}"
  local target="${3:?missing target file}"
  install -D -m 0644 "$src" "$target"
}

copy_file() {
  local src="${2:?missing source file}"
  local target="${3:?missing target file}"
  install -D -m 0644 "$src" "$target"
}

delete_file() {
  local target="${2:?missing target file}"
  rm -f "$target"
}

upgrade_core() {
  local latest_ver ver_num arch arch_val download_url
  latest_ver=$(curl -fsSLI -o /dev/null -w '%{url_effective}' https://github.com/SagerNet/sing-box/releases/latest | awk -F/ '{print $NF}')
  [[ -z "$latest_ver" ]] && echo "failed to get latest version" && exit 1
  arch=$(uname -m)
  case $arch in
    x86_64) arch_val="amd64" ;;
    aarch64) arch_val="arm64" ;;
    *) echo "unsupported arch: $arch"; exit 1 ;;
  esac
  ver_num=${latest_ver#v}
  download_url="https://github.com/SagerNet/sing-box/releases/download/${latest_ver}/sing-box-${ver_num}-linux-${arch_val}.tar.gz"
  wget -O /tmp/sing-box.tar.gz "$download_url"
  systemctl stop "$SERVICE_NAME"
  tar -zxvf /tmp/sing-box.tar.gz -C /tmp/ >/dev/null
  mv "/tmp/sing-box-${ver_num}-linux-${arch_val}/sing-box" "$BIN_PATH"
  chmod +x "$BIN_PATH"
  rm -rf /tmp/sing-box* 
  systemctl start "$SERVICE_NAME"
  "$BIN_PATH" version
}

cmd="${1:-}"
case "$cmd" in
  status) systemctl status "$SERVICE_NAME" --no-pager ;;
  start|stop|restart|enable|disable) systemctl "$cmd" "$SERVICE_NAME" ;;
  get-url) cat "$URL_FILE" ;;
  set-url) printf '%s\n' "${2:?missing url}" > "$URL_FILE" ;;
  update) safe_update ;;
  get-config) cat "$CONF_FILE" ;;
  save-config) save_config "$@" ;;
  write-file) write_file "$@" ;;
  copy-file) copy_file "$@" ;;
  delete-file) delete_file "$@" ;;
  create-backup)
    cp "$CONF_FILE" "${CONF_FILE}.backup.$(date +%Y%m%d-%H%M%S)"
    ;;
  delete-backup)
    name="${2:?missing backup name}"
    rm -f "${CONF_DIR}/${name}"
    ;;
  check-config) "$BIN_PATH" check -c "${2:?missing file}" ;;
  version) "$BIN_PATH" version ;;
  latest-version)
    curl -fsSLI -o /dev/null -w '%{url_effective}' https://github.com/SagerNet/sing-box/releases/latest | awk -F/ '{print $NF}'
    ;;
  cron-show) cron_line ;;
  cron-summary) cron_summary ;;
  cron-set)
    days="${2:?missing days}"; hour="${3:?missing hour}"
    crontab -l 2>/dev/null | grep -v "$SCRIPT_PATH update" | crontab - || true
    if [[ "$days" == "1" ]]; then expr="0 $hour * * *"; else expr="0 $hour */$days * *"; fi
    (crontab -l 2>/dev/null; echo "$expr /bin/bash $SCRIPT_PATH update >/dev/null 2>&1") | crontab -
    ;;
  cron-delete) crontab -l 2>/dev/null | grep -v "$SCRIPT_PATH update" | crontab - || true ;;
  upgrade) upgrade_core ;;
  *) echo "usage: $0 {status|start|stop|restart|enable|disable|get-url|set-url|update|get-config|save-config|write-file|copy-file|delete-file|create-backup|delete-backup|check-config|version|latest-version|cron-show|cron-summary|cron-set|cron-delete|upgrade}"; exit 1 ;;
esac
