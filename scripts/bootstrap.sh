#!/usr/bin/env bash
set -Eeuo pipefail

# Скрипт начальной подготовки контуров.
# Он ничего не запускает, а только создает нужные каталоги на хосте.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Использование: ./scripts/bootstrap.sh [dev|prod]

Скрипт подготавливает каталоги хранения данных и бэкапов для выбранного контура.
EOF
}

# Определяем контур.
parse_contour_arg "$@"
if [[ ${#POSITIONAL_ARGS[@]} -ne 0 ]]; then
  usage >&2
  exit 1
fi

# Загружаем его env-файл и создаем нужные директории.
resolve_env_file
load_env
ensure_runtime_dirs

# Печатаем итог для администратора.
print_context
echo "Каталоги подготовлены:"
echo "  База данных: $(root_path "$DB_STORAGE_DIR")"
echo "  Файлы приложения: $(root_path "$ESPO_STORAGE_DIR")"
echo "  Бэкапы: $(root_path "$BACKUP_ROOT")"
