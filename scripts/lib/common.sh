#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

die() {
  echo "Error: $*" >&2
  exit 1
}

usage_error() {
  if declare -F usage >/dev/null 2>&1; then
    usage >&2
  fi
  die "$@"
}

ensure_espops_binary() {
  local bin_path="${1:-$ROOT_DIR/bin/espops}"

  if [[ -x "$bin_path" ]]; then
    return 0
  fi

  die "A prebuilt Go CLI is required: $bin_path. Build it explicitly with make build or set ESPOPS_BIN"
}

run_espops() {
  local bin_path="${ESPOPS_BIN:-$ROOT_DIR/bin/espops}"

  ensure_espops_binary "$bin_path"
  "$bin_path" "$@"
}
