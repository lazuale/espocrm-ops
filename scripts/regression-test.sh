#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"

usage() {
  cat <<'EOF'
Usage: ./scripts/regression-test.sh

This script runs a minimal regression suite for the retained backup/recovery shell wrappers.
EOF
}

TEST_TMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/espo-regression.XXXXXX")"
JSON_FLAG="--json"
trap 'rm -rf -- "$TEST_TMP_ROOT"' EXIT

announce_test() {
  printf '\n== %s ==\n' "$1"
}

fail_test() {
  echo "[error] $*" >&2
  exit 1
}

pass_test() {
  echo "[ok] $*"
}

run_command_capture() {
  local output_file="$1"
  shift

  set +e
  "$@" >"$output_file" 2>&1
  local status=$?
  set -e
  return "$status"
}

assert_file_contains() {
  local file="$1"
  local expected="$2"
  local label="$3"

  if ! rg -Fq -- "$expected" "$file"; then
    echo "[debug] File output $file:" >&2
    sed -n '1,220p' "$file" >&2 || true
    fail_test "$label"
  fi
}

assert_file_equals() {
  local file="$1"
  local expected_file="$2"
  local label="$3"

  if ! cmp -s "$file" "$expected_file"; then
    echo "[debug] Expected output $expected_file:" >&2
    sed -n '1,220p' "$expected_file" >&2 || true
    echo "[debug] Actual output $file:" >&2
    sed -n '1,220p' "$file" >&2 || true
    fail_test "$label"
  fi
}

copy_example_env() {
  local contour="$1"
  local target="$2"

  cp "$ROOT_DIR/ops/env/.env.${contour}.example" "$target"
  chmod 600 "$target"
}

create_mock_espops() {
  local target="$1"

  cat > "$target" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
printf 'ARGS:'
for arg in "$@"; do
  printf ' [%s]' "$arg"
done
printf '\n'

if [[ "${1:-}" == "--json" ]]; then
  shift
  printf '{"ok":true,"command":"%s"}\n' "$1"
fi
EOF
  chmod +x "$target"
}

test_doctor_wrapper_passes_through_json() {
  announce_test "doctor wrapper"

  local env_file="$TEST_TMP_ROOT/doctor.env"
  local output_file="$TEST_TMP_ROOT/doctor.out"
  local mock_espops="$TEST_TMP_ROOT/mock.doctor.espops"

  copy_example_env dev "$env_file"
  create_mock_espops "$mock_espops"

  if ! run_command_capture "$output_file" env ENV_FILE="$env_file" ESPOPS_BIN="$mock_espops" bash "$SCRIPT_DIR/doctor.sh" dev "$JSON_FLAG"; then
    fail_test "doctor wrapper failed"
  fi

  assert_file_contains "$output_file" "ARGS: [$JSON_FLAG] [doctor] [--scope] [dev] [--project-dir] [$ROOT_DIR] [--compose-file] [$ROOT_DIR/compose.yaml] [--env-file] [$env_file]" "doctor args mismatch"
  assert_file_contains "$output_file" '{"ok":true,"command":"doctor"}' "doctor json mismatch"
  pass_test "doctor wrapper passed"
}

test_backup_wrapper_delegates_to_go_backup() {
  announce_test "backup wrapper"

  local env_file="$TEST_TMP_ROOT/backup.env"
  local output_file="$TEST_TMP_ROOT/backup.out"
  local mock_espops="$TEST_TMP_ROOT/mock.backup.espops"

  copy_example_env dev "$env_file"
  create_mock_espops "$mock_espops"

  if ! run_command_capture "$output_file" env ENV_FILE="$env_file" ESPOPS_BIN="$mock_espops" bash "$SCRIPT_DIR/backup.sh" dev --skip-files --no-stop; then
    fail_test "backup wrapper failed"
  fi

  assert_file_contains "$output_file" "ARGS: [backup] [--scope] [dev] [--project-dir] [$ROOT_DIR] [--compose-file] [$ROOT_DIR/compose.yaml] [--env-file] [$env_file] [--skip-files] [--no-stop]" "backup args mismatch"
  pass_test "backup wrapper passed"
}

test_backup_verify_wrapper_resolves_backup_root() {
  announce_test "backup verify wrapper"

  local env_file="$TEST_TMP_ROOT/verify.env"
  local output_file="$TEST_TMP_ROOT/verify.out"
  local mock_espops="$TEST_TMP_ROOT/mock.verify.espops"
  local backup_root="$TEST_TMP_ROOT/backups/dev"

  copy_example_env dev "$env_file"
  mkdir -p "$backup_root"
  set_env_value "$env_file" BACKUP_ROOT "$backup_root"
  create_mock_espops "$mock_espops"

  if ! run_command_capture "$output_file" env ENV_FILE="$env_file" ESPOPS_BIN="$mock_espops" bash "$SCRIPT_DIR/backup.sh" verify dev "$JSON_FLAG"; then
    fail_test "backup verify wrapper failed"
  fi

  assert_file_contains "$output_file" "ARGS: [backup] [verify] [--backup-root] [$backup_root] [$JSON_FLAG]" "backup verify args mismatch"
  pass_test "backup verify wrapper passed"
}

test_restore_wrapper_delegates_to_go_restore() {
  announce_test "restore wrapper"

  local env_file="$TEST_TMP_ROOT/restore.env"
  local output_file="$TEST_TMP_ROOT/restore.out"
  local mock_espops="$TEST_TMP_ROOT/mock.restore.espops"

  copy_example_env prod "$env_file"
  create_mock_espops "$mock_espops"

  if ! run_command_capture "$output_file" env ENV_FILE="$env_file" ESPOPS_BIN="$mock_espops" bash "$SCRIPT_DIR/restore.sh" prod --manifest /tmp/manifest.json --force --confirm-prod prod; then
    fail_test "restore wrapper failed"
  fi

  assert_file_contains "$output_file" "ARGS: [restore] [--scope] [prod] [--project-dir] [$ROOT_DIR] [--compose-file] [$ROOT_DIR/compose.yaml] [--env-file] [$env_file] [--manifest] [/tmp/manifest.json] [--force] [--confirm-prod] [prod]" "restore args mismatch"
  pass_test "restore wrapper passed"
}

test_migrate_wrapper_delegates_to_go_migrate() {
  announce_test "migrate wrapper"

  local output_file="$TEST_TMP_ROOT/migrate.out"
  local mock_espops="$TEST_TMP_ROOT/mock.migrate.espops"

  create_mock_espops "$mock_espops"

  if ! run_command_capture "$output_file" env ESPOPS_BIN="$mock_espops" bash "$SCRIPT_DIR/migrate.sh" dev prod --force --confirm-prod prod; then
    fail_test "migrate wrapper failed"
  fi

  assert_file_contains "$output_file" "ARGS: [migrate] [--from] [dev] [--to] [prod] [--project-dir] [$ROOT_DIR] [--compose-file] [$ROOT_DIR/compose.yaml] [--force] [--confirm-prod] [prod]" "migrate args mismatch"
  pass_test "migrate wrapper passed"
}

test_dispatcher_help_shows_only_core_commands() {
  announce_test "dispatcher help"

  local output_file="$TEST_TMP_ROOT/espo-help.out"
  local expected_file="$TEST_TMP_ROOT/espo-help.expected"

  if ! run_command_capture "$output_file" bash "$SCRIPT_DIR/espo.sh" help; then
    fail_test "dispatcher help failed"
  fi

  cat > "$expected_file" <<'EOF'
Usage: ./scripts/espo.sh <command> [arguments...]

Retained operator-facing commands:
  doctor [dev|prod|all]          Check readiness before backup or recovery work
  backup <dev|prod> [args...]    Create a backup
  backup verify <dev|prod>       Verify the latest backup set for a contour
  restore <dev|prod> [args...]   Restore from a backup
  migrate <from> <to> [args...]  Migrate a backup between contours
  help [command]                 Show general help or command help
EOF
  assert_file_equals "$output_file" "$expected_file" "dispatcher help mismatch"
  pass_test "dispatcher help passed"
}

main() {
  cd "$ROOT_DIR"

  if [[ $# -gt 0 ]]; then
    case "$1" in
      -h|--help)
        usage
        exit 0
        ;;
      *)
        usage >&2
        die "Unknown argument: $1"
        ;;
    esac
  fi

  test_doctor_wrapper_passes_through_json
  test_backup_wrapper_delegates_to_go_backup
  test_backup_verify_wrapper_resolves_backup_root
  test_restore_wrapper_delegates_to_go_restore
  test_migrate_wrapper_delegates_to_go_migrate
  test_dispatcher_help_shows_only_core_commands

  printf '\nRegression suite completed successfully\n'
}

main "$@"
