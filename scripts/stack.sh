#!/usr/bin/env bash
set -Eeuo pipefail

# Подключаем общие функции:
# - определение контура;
# - выбор env-файла;
# - единый вызов docker compose.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Использование: ./scripts/stack.sh [dev|prod] <аргументы-docker-compose...>

Примеры:
  ./scripts/stack.sh prod up -d
  ./scripts/stack.sh dev ps
  ./scripts/stack.sh prod logs -f espocrm
EOF
}

# Отделяем имя контура от остальных аргументов.
parse_contour_arg "$@"
if [[ ${#POSITIONAL_ARGS[@]} -eq 0 ]]; then
  usage >&2
  exit 1
fi

# Подготавливаем окружение для корректного вызова docker compose.
resolve_env_file
load_env
require_compose

# Просто прокидываем оставшиеся аргументы в compose.
compose "${POSITIONAL_ARGS[@]}"
