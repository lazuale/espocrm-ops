#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

die() {
  echo "Error: $*" >&2
  exit 1
}

run_script() {
  local script_name="$1"
  shift

  [[ -f "$SCRIPT_DIR/$script_name" ]] || die "Script not found: $SCRIPT_DIR/$script_name"
  bash "$SCRIPT_DIR/$script_name" "$@"
}

show_compat_script_help() {
  local dispatcher_usage="$1"
  local script_name="$2"

  cat <<EOF
Unified operator entrypoint:
  $dispatcher_usage

Compatible direct entrypoint:
  ./scripts/$script_name [args...]

Compatible script help follows:
EOF
  run_script "$script_name" -h
}

usage() {
  cat <<'EOF'
Usage: ./scripts/espo.sh <command> [arguments...]

When in doubt, the safe order is usually:
  doctor -> status -> backup -> update/restore/rollback

Daily commands:
  doctor [dev|prod|all]          Check env, Docker, ports, and config before changes
  overview <dev|prod> [args...]  Show a canonical contour summary of readiness, runtime, backups, and recent operations
  status <dev|prod> [args...]    Get a short report about the current contour state
  backup <dev|prod> [args...]    Create a new coherent backup set
  support <dev|prod> [args...]   Collect a diagnostic bundle for troubleshooting

Core commands:
  help [command...]              Show general help or help for a specific command
  bootstrap <dev|prod>           Prepare directories for runtime data, backups, and reports
  stack <dev|prod> <args...>     Direct access to the compose wrapper when precise control is needed

Stack shortcuts:
  up|down|stop|restart|ps|logs|pull|config|exec <dev|prod> [args...]
                                  Convenient shortcuts instead of longer stack invocations

Backups:
  backup verify <dev|prod> [args...]
                                  Verify a specific backup set
  backup audit <dev|prod> [args...]
                                  Check backup freshness and integrity
  backup catalog <dev|prod> [args...]
                                  List available backup sets and select a valid one

Restore:
  restore db <dev|prod> <file> [args...]
                                  Restore only the database
  restore files <dev|prod> <file> [args...]
                                  Restore only the file storage
  restore drill <dev|prod> [args...]
                                  Check backup recoverability in a temporary contour

Other:
  rollback <dev|prod> [args...]  Return to the latest coherent backup set
  migrate <from> <to> [args...]  Migrate a backup between contours
  update <dev|prod> [args...]    Run a standard update with readiness checks
  smoke <dev|prod> [args...]     Run a quick isolated startup test
  cleanup [args...]              Preview or apply safe Docker host cleanup
  regression [args...]           Run the internal toolkit regression suite

Repository bootstrap:
  AGENTS.md

Focused help:
  ./scripts/espo.sh help backup
  ./scripts/espo.sh help restore db
  ./scripts/espo.sh help update
EOF
}

run_stack_shortcut() {
  local action="$1"
  shift || true

  if [[ "${1:-}" != "dev" && "${1:-}" != "prod" ]]; then
    usage >&2
    die "Command $action requires an explicit dev or prod contour"
  fi

  local contour="$1"
  shift
  run_script stack.sh "$contour" "$action" "$@"
}

run_backup_namespace() {
  local subcommand="${1:-}"

  case "$subcommand" in
    ""|dev|prod|--*)
      run_script backup.sh "$@"
      ;;
    verify)
      shift
      run_script verify-backup.sh "$@"
      ;;
    audit)
      shift
      run_script backup-audit.sh "$@"
      ;;
    catalog)
      shift
      run_script backup-catalog.sh "$@"
      ;;
    *)
      usage >&2
      die "Unknown backup subcommand: $subcommand"
      ;;
  esac
}

run_restore_namespace() {
  local subcommand="${1:-}"

  case "$subcommand" in
    db)
      shift
      run_script restore-db.sh "$@"
      ;;
    files)
      shift
      run_script restore-files.sh "$@"
      ;;
    drill)
      shift
      run_script restore-drill.sh "$@"
      ;;
    *)
      usage >&2
      die "Unknown restore subcommand: $subcommand"
      ;;
  esac
}

show_help() {
  local command="${1:-}"
  local subcommand="${2:-}"

  case "$command" in
    ""|help|-h|--help)
      usage
      ;;
    doctor)
      show_compat_script_help "./scripts/espo.sh doctor [dev|prod|all] [args...]" doctor.sh
      ;;
    bootstrap)
      show_compat_script_help "./scripts/espo.sh bootstrap <dev|prod> [args...]" bootstrap.sh
      ;;
    overview)
      show_compat_script_help "./scripts/espo.sh overview <dev|prod> [args...]" contour-overview.sh
      ;;
    stack)
      show_compat_script_help "./scripts/espo.sh stack <dev|prod> <args...>" stack.sh
      ;;
    up|down|stop|restart|ps|logs|pull|config|exec)
      usage
      ;;
    backup)
      case "$subcommand" in
        verify)
          show_compat_script_help "./scripts/espo.sh backup verify <dev|prod> [args...]" verify-backup.sh
          ;;
        audit)
          show_compat_script_help "./scripts/espo.sh backup audit <dev|prod> [args...]" backup-audit.sh
          ;;
        catalog)
          show_compat_script_help "./scripts/espo.sh backup catalog <dev|prod> [args...]" backup-catalog.sh
          ;;
        *)
          show_compat_script_help "./scripts/espo.sh backup <dev|prod> [args...]" backup.sh
          ;;
      esac
      ;;
    restore)
      case "$subcommand" in
        db)
          show_compat_script_help "./scripts/espo.sh restore db <dev|prod> <file> [args...]" restore-db.sh
          ;;
        files)
          show_compat_script_help "./scripts/espo.sh restore files <dev|prod> <file> [args...]" restore-files.sh
          ;;
        drill)
          show_compat_script_help "./scripts/espo.sh restore drill <dev|prod> [args...]" restore-drill.sh
          ;;
        *)
          usage
          ;;
      esac
      ;;
    rollback)
      show_compat_script_help "./scripts/espo.sh rollback <dev|prod> [args...]" rollback.sh
      ;;
    migrate)
      show_compat_script_help "./scripts/espo.sh migrate <from> <to> [args...]" migrate-backup.sh
      ;;
    status)
      show_compat_script_help "./scripts/espo.sh status <dev|prod> [args...]" status-report.sh
      ;;
    support)
      show_compat_script_help "./scripts/espo.sh support <dev|prod> [args...]" support-bundle.sh
      ;;
    update)
      show_compat_script_help "./scripts/espo.sh update <dev|prod> [args...]" update.sh
      ;;
    smoke)
      show_compat_script_help "./scripts/espo.sh smoke <dev|prod> [args...]" smoke-test.sh
      ;;
    cleanup)
      show_compat_script_help "./scripts/espo.sh cleanup [args...]" docker-cleanup.sh
      ;;
    regression)
      show_compat_script_help "./scripts/espo.sh regression [args...]" regression-test.sh
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
  bootstrap)
    shift
    run_script bootstrap.sh "$@"
    ;;
  overview)
    shift
    run_script contour-overview.sh "$@"
    ;;
  stack)
    shift
    run_script stack.sh "$@"
    ;;
  up|down|stop|restart|ps|logs|pull|config|exec)
    action="$COMMAND"
    shift
    run_stack_shortcut "$action" "$@"
    ;;
  backup)
    shift
    run_backup_namespace "$@"
    ;;
  restore)
    shift
    run_restore_namespace "$@"
    ;;
  rollback)
    shift
    run_script rollback.sh "$@"
    ;;
  migrate)
    shift
    run_script migrate-backup.sh "$@"
    ;;
  status)
    shift
    run_script status-report.sh "$@"
    ;;
  support)
    shift
    run_script support-bundle.sh "$@"
    ;;
  update)
    shift
    run_script update.sh "$@"
    ;;
  smoke)
    shift
    run_script smoke-test.sh "$@"
    ;;
  cleanup)
    shift
    run_script docker-cleanup.sh "$@"
    ;;
  regression)
    shift
    run_script regression-test.sh "$@"
    ;;
  *)
    usage >&2
    die "Unknown command: $COMMAND"
    ;;
esac
