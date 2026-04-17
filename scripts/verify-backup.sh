#!/usr/bin/env bash
set -Eeuo pipefail

# Full backup-set verification is Go-owned. Shell keeps contour/env handling
# and legacy partial checksum-only checks for --skip-db / --skip-files.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/locks.sh"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/artifacts.sh"

usage() {
  cat <<'EOF'
Usage:
  ./scripts/verify-backup.sh /path/to/manifest.json
  ./scripts/verify-backup.sh <dev|prod> [--manifest PATH | --db-backup PATH --files-backup PATH | --db-backup PATH | --files-backup PATH] [--skip-db] [--skip-files]

Examples:
  ./scripts/verify-backup.sh /opt/espocrm-data/backups/prod/manifests/espocrm-prod_YYYY-MM-DD_HH-MM-SS.manifest.json
  ./scripts/verify-backup.sh prod
  ./scripts/verify-backup.sh dev
  ./scripts/verify-backup.sh prod --db-backup /opt/espocrm-data/backups/prod/db/espocrm-prod_YYYY-MM-DD_HH-MM-SS.sql.gz
  ./scripts/verify-backup.sh dev --files-backup /opt/espocrm-data/backups/dev/files/espocrm-dev_files_YYYY-MM-DD_HH-MM-SS.tar.gz
EOF
}

verify_required_sidecar() {
  local label="$1"
  local backup_file="$2"
  local sidecar_file="${backup_file}.sha256"

  [[ -f "$backup_file" ]] || die "Backup file for verification not found: $backup_file"
  [[ -f "$sidecar_file" ]] || die "Checksum file for verification not found: $sidecar_file"

  if verify_sha256_sidecar "$backup_file" "$sidecar_file"; then
    echo "[ok] $label: checksum verified"
    echo "      File: $backup_file"
    echo "      SHA:  $(read_sha256_sidecar "$sidecar_file")"
  else
    die "Checksum mismatch for file: $backup_file"
  fi
}

resolve_manifest_for_selected_pair() {
  local prefix stamp

  if [[ -n "$DB_BACKUP" ]]; then
    IFS='|' read -r prefix stamp < <(backup_group_from_db_file "$DB_BACKUP") || return 1
  else
    IFS='|' read -r prefix stamp < <(backup_group_from_files_file "$FILES_BACKUP") || return 1
  fi

  IFS='|' read -r _db _files _manifest_txt MANIFEST_JSON < <(
    backup_set_paths "$BACKUP_ROOT_ABS" "$prefix" "$stamp"
  )
}

resolve_full_manifest_selection() {
  if [[ -n "$MANIFEST_JSON" ]]; then
    return
  fi

  if [[ -n "$DB_BACKUP" && -z "$FILES_BACKUP" ]]; then
    FILES_BACKUP="$(matching_files_backup_for_db "$BACKUP_ROOT_ABS" "$DB_BACKUP" || true)"
    [[ -n "$FILES_BACKUP" && -f "$FILES_BACKUP" ]] || die "No coherent files backup found for verification; pass --files-backup explicitly if needed"
  fi

  if [[ -z "$DB_BACKUP" && -n "$FILES_BACKUP" ]]; then
    DB_BACKUP="$(matching_db_backup_for_files "$BACKUP_ROOT_ABS" "$FILES_BACKUP" || true)"
    [[ -n "$DB_BACKUP" && -f "$DB_BACKUP" ]] || die "No coherent database backup found for verification; pass --db-backup explicitly if needed"
  fi

  backup_pair_is_coherent "$DB_BACKUP" "$FILES_BACKUP" || die "The database and files backups used for verification must belong to the same backup set"
  resolve_manifest_for_selected_pair || die "Could not resolve the JSON manifest for the selected backup pair"
}

resolve_partial_selection() {
  if [[ $SKIP_DB -eq 0 && -z "$DB_BACKUP" ]]; then
    DB_BACKUP="$(latest_backup_file "$BACKUP_ROOT_ABS/db" '*.sql.gz' || true)"
  fi

  if [[ $SKIP_FILES -eq 0 && -z "$FILES_BACKUP" ]]; then
    FILES_BACKUP="$(latest_backup_file "$BACKUP_ROOT_ABS/files" '*.tar.gz' || true)"
  fi
}

run_full_go_verify() {
  if [[ -n "$MANIFEST_JSON" ]]; then
    [[ -f "$MANIFEST_JSON" ]] || die "JSON manifest not found: $MANIFEST_JSON"
    run_espops verify-backup --manifest "$MANIFEST_JSON"
  else
    run_espops verify-backup --backup-root "$BACKUP_ROOT_ABS"
  fi
}

if [[ $# -gt 0 && "${1:-}" != "dev" && "${1:-}" != "prod" && "${1:-}" != "-h" && "${1:-}" != "--help" ]]; then
  [[ $# -eq 1 ]] || { usage >&2; die "Direct manifest verification accepts only a path to manifest.json"; }
  MANIFEST_JSON="$(caller_path "$1")"
  [[ -f "$MANIFEST_JSON" ]] || die "JSON manifest not found: $MANIFEST_JSON"
  run_espops verify-backup --manifest "$MANIFEST_JSON"
  exit $?
fi

parse_contour_arg "$@"

DB_BACKUP=""
FILES_BACKUP=""
MANIFEST_JSON=""
SKIP_DB=0
SKIP_FILES=0

while [[ ${#POSITIONAL_ARGS[@]} -gt 0 ]]; do
  case "${POSITIONAL_ARGS[0]}" in
    --db-backup)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "--db-backup must be followed by a path"
      DB_BACKUP="$(caller_path "${POSITIONAL_ARGS[1]}")"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
      ;;
    --files-backup)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "--files-backup must be followed by a path"
      FILES_BACKUP="$(caller_path "${POSITIONAL_ARGS[1]}")"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
      ;;
    --manifest)
      [[ ${#POSITIONAL_ARGS[@]} -ge 2 ]] || die "--manifest must be followed by a path"
      MANIFEST_JSON="$(caller_path "${POSITIONAL_ARGS[1]}")"
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:2}")
      ;;
    --skip-db)
      SKIP_DB=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    --skip-files)
      SKIP_FILES=1
      POSITIONAL_ARGS=("${POSITIONAL_ARGS[@]:1}")
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      die "Unknown argument: ${POSITIONAL_ARGS[0]}"
      ;;
  esac
done

require_explicit_contour

if [[ $SKIP_DB -eq 1 && $SKIP_FILES -eq 1 ]]; then
  die "Nothing to verify: --skip-db and --skip-files cannot both be set"
fi

if [[ -n "$MANIFEST_JSON" && ( $SKIP_DB -eq 1 || $SKIP_FILES -eq 1 ) ]]; then
  die "--manifest is incompatible with partial verification via --skip-db or --skip-files"
fi

acquire_operation_lock verify-backup
resolve_env_file
load_env
ensure_runtime_dirs

BACKUP_ROOT_ABS="$(root_path "$BACKUP_ROOT")"

if [[ $SKIP_DB -eq 0 && $SKIP_FILES -eq 0 ]]; then
  if [[ -n "$DB_BACKUP" || -n "$FILES_BACKUP" ]]; then
    resolve_full_manifest_selection
  fi
  run_full_go_verify
  exit $?
fi

resolve_partial_selection
print_context

if [[ $SKIP_DB -eq 0 ]]; then
  [[ -n "$DB_BACKUP" ]] || die "Database backup for verification not found"
  verify_required_sidecar "Database" "$DB_BACKUP"
fi

if [[ $SKIP_FILES -eq 0 ]]; then
  [[ -n "$FILES_BACKUP" ]] || die "Files backup for verification not found"
  verify_required_sidecar "Files" "$FILES_BACKUP"
fi

echo "Backup file verification completed successfully"
