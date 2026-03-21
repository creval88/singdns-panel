#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${1:-http://127.0.0.1:9999}"
COOKIE_JAR="${COOKIE_JAR:-/tmp/singdns-panel-cookie.txt}"
CSRF_HEADER="${CSRF_HEADER:-x-csrf-token}"

if [[ -z "${PANEL_USER:-}" || -z "${PANEL_PASS:-}" ]]; then
  echo "需要先设置 PANEL_USER/PANEL_PASS 环境变量"
  echo "示例: PANEL_USER=admin PANEL_PASS=123456 $0 http://127.0.0.1:9999"
  exit 1
fi

echo "[1/5] 登录获取 session..."
LOGIN_JSON=$(curl -sS -c "$COOKIE_JAR" -b "$COOKIE_JAR" -H 'content-type: application/json' \
  -X POST "$BASE_URL/api/login" \
  --data "{\"username\":\"$PANEL_USER\",\"password\":\"$PANEL_PASS\"}")
echo "$LOGIN_JSON" | grep -q '"ok":true' || { echo "登录失败: $LOGIN_JSON"; exit 1; }

echo "[2/5] 拉取 csrf token..."
CSRF=$(curl -sS -c "$COOKIE_JAR" -b "$COOKIE_JAR" "$BASE_URL/api/csrf" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
[[ -n "$CSRF" ]] || { echo "获取 csrf 失败"; exit 1; }

echo "[3/5] 保存 update-config（示例: beta+amd64）..."
SAVE_JSON=$(curl -sS -c "$COOKIE_JAR" -b "$COOKIE_JAR" -H 'content-type: application/json' -H "$CSRF_HEADER: $CSRF" \
  -X POST "$BASE_URL/api/panel/update-config" \
  --data '{"channel":"beta","arch":"amd64","base_url":"https://raw.githubusercontent.com/creval88/singdns-panel/main/updates/latest.json"}')
echo "$SAVE_JSON" | grep -q '"ok":true' || { echo "保存失败: $SAVE_JSON"; exit 1; }

echo "[4/5] 探测 remote..."
PROBE_JSON=$(curl -sS -c "$COOKIE_JAR" -b "$COOKIE_JAR" "$BASE_URL/api/panel/probe-remote")
echo "$PROBE_JSON"

echo "[5/5] 查询 version..."
VER_JSON=$(curl -sS -c "$COOKIE_JAR" -b "$COOKIE_JAR" "$BASE_URL/api/panel/version")
echo "$VER_JSON" | grep -q '"checked_at"' || { echo "version 缺少 checked_at: $VER_JSON"; exit 1; }
echo "$VER_JSON"

echo "OK: panel update APIs smoke passed"
