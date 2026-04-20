#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

run_script() {
  local script_name="$1"
  shift

  [[ -f "$SCRIPT_DIR/$script_name" ]] || die "Script not found: $SCRIPT_DIR/$script_name"
  bash "$SCRIPT_DIR/$script_name" "$@"
}

usage() {
  cat <<'EOF'
Usage: ./scripts/espo.sh <command> [arguments...]

Retained operator-facing commands:
  doctor <dev|prod|all>          Check readiness before backup or recovery work
  backup <dev|prod> [args...]    Create a backup
  backup verify [source args...] Verify a backup set from an explicit source
  restore <dev|prod> [args...]   Restore from a backup
  migrate <from> <to> [args...]  Migrate a backup between contours
  help [command]                 Show general help or command help
EOF
}

show_help() {
  local command="${1:-}"

  case "$command" in
    ""|help|-h|--help)
      usage
      ;;
    doctor)
      run_script doctor.sh --help
      ;;
    backup)
      run_script backup.sh --help
      ;;
    restore)
      run_script restore.sh --help
      ;;
    migrate)
      run_script migrate.sh --help
      ;;
    *)
      usage >&2
      die "Unknown help command: $command"
      ;;
  esac
}

COMMAND="${1:-help}"

case "$COMMAND" in
  help|-h|--help)
    shift || true
    show_help "$@"
    ;;
  doctor)
    shift
    run_script doctor.sh "$@"
    ;;
  backup)
    shift
    run_script backup.sh "$@"
    ;;
  restore)
    shift
    run_script restore.sh "$@"
    ;;
  migrate)
    shift
    run_script migrate.sh "$@"
    ;;
  *)
    usage >&2
    die "Unknown command: $COMMAND"
    ;;
esac
