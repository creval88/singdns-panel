#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
INTERNAL_DIR="$ROOT_DIR/internal/webassets"
WEB_DIR="$ROOT_DIR/web"

usage() {
  cat <<'USAGE'
Usage: scripts/check-webassets-parity.sh [--sync]

Checks whether the editable web mirror matches internal/webassets, which is the
asset tree embedded into the shipped binary.

Options:
  --sync    Copy templates/static from internal/webassets to web before checking.
USAGE
}

SYNC=0
case "${1:-}" in
  --sync)
    SYNC=1
    ;;
  -h|--help)
    usage
    exit 0
    ;;
  "")
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac

if [[ ! -d "$INTERNAL_DIR/templates" || ! -d "$INTERNAL_DIR/static" ]]; then
  echo "missing internal webassets tree: $INTERNAL_DIR" >&2
  exit 1
fi

if [[ "$SYNC" == "1" ]]; then
  mkdir -p "$WEB_DIR/templates" "$WEB_DIR/static"
  rsync -a --delete "$INTERNAL_DIR/templates/" "$WEB_DIR/templates/"
  rsync -a --delete "$INTERNAL_DIR/static/" "$WEB_DIR/static/"
fi

status=0
for subdir in templates static; do
  if ! diff -qr "$INTERNAL_DIR/$subdir" "$WEB_DIR/$subdir"; then
    status=1
  fi
done

if [[ "$status" == "0" ]]; then
  echo "web asset mirror is in sync"
else
  echo "web asset mirror differs from internal/webassets" >&2
fi
exit "$status"
