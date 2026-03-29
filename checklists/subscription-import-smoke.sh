#!/usr/bin/env bash
set -euo pipefail

PROJECT_DIR="${PROJECT_DIR:-$(cd "$(dirname "$0")/.." && pwd)}"
SUB_URL="${1:-${SUB_URL:-}}"
CFG_ARG="${2:-configs/panel.json}"

if [[ -z "$SUB_URL" ]]; then
  echo "usage: $0 <subscription-url> [config-path]"
  exit 2
fi

cd "$PROJECT_DIR"

echo "[1/5] pwd"
pwd

echo "[2/5] go env sanity"
go env GOMOD GOWORK GOPATH GO111MODULE

echo "[3/5] test+build"
go test ./...
go build ./cmd/server

echo "[4/5] run import"
GO111MODULE=on go run ./cmd/server subscription-import "$SUB_URL" "$CFG_ARG"

echo "[5/5] resolve output config path"
OUT_PATH="$(sed -n 's/.*"config_path"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$CFG_ARG" | head -n1)"
if [[ -z "$OUT_PATH" ]]; then
  echo "cannot resolve services.singbox.config_path from $CFG_ARG" >&2
  exit 1
fi

echo "output config path: $OUT_PATH"
ls -l "$OUT_PATH"
grep -n '"{all}"' "$OUT_PATH" || true
sing-box check -c "$OUT_PATH"

echo "OK"
