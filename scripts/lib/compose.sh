#!/usr/bin/env bash
set -Eeuo pipefail

require_compose() {
  local skip_daemon_check=0

  case "${1:-}" in
    "")
      ;;
    --skip-daemon-check)
      skip_daemon_check=1
      ;;
    *)
      die "Unknown require_compose argument: $1"
      ;;
  esac

  command -v docker >/dev/null 2>&1 || die "docker is not installed or not found in PATH"
  docker compose version >/dev/null 2>&1 || die "the docker compose plugin is unavailable"

  if [[ $skip_daemon_check -eq 0 ]]; then
    require_docker_daemon
  fi
}

require_docker_daemon() {
  local output

  command -v docker >/dev/null 2>&1 || die "docker is not installed or not found in PATH"

  if ! output="$(docker info 2>&1)"; then
    output="$(printf '%s\n' "$output" | awk 'NF { last=$0 } END { print last }')"
    [[ -n "$output" ]] || output="unknown error"
    die "Docker daemon is unavailable. Check that Docker is running and that the current user can access /var/run/docker.sock. Details: $output"
  fi
}

compose() {
  docker compose \
    --project-directory "$ROOT_DIR" \
    -f "$ROOT_DIR/compose.yaml" \
    --env-file "$ENV_FILE" \
    "$@"
}

require_service_running() {
  local service="$1"

  if ! compose ps --status running --services | grep -qx "$service"; then
    die "Service '$service' is not running for contour '$ESPO_ENV'"
  fi
}

service_is_running() {
  local service="$1"
  compose ps --status running --services | grep -qx "$service"
}

app_services_running() {
  local running_services
  running_services="$(compose ps --status running --services || true)"

  grep -qx 'espocrm' <<<"$running_services" \
    || grep -qx 'espocrm-daemon' <<<"$running_services" \
    || grep -qx 'espocrm-websocket' <<<"$running_services"
}

stop_app_services() {
  compose stop espocrm espocrm-daemon espocrm-websocket >/dev/null 2>&1 || true
}

start_app_services() {
  compose up -d espocrm espocrm-daemon espocrm-websocket
}

compose_service_container_id() {
  local service="$1"
  compose ps -q "$service"
}

compose_service_status() {
  local service="$1"
  local container_id
  container_id="$(compose_service_container_id "$service")"

  [[ -n "$container_id" ]] || return 1

  docker inspect \
    --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' \
    "$container_id"
}

current_epoch_seconds() {
  date +%s
}

sleep_for_readiness_poll() {
  local remaining_seconds="$1"
  local sleep_seconds=5

  if (( remaining_seconds < sleep_seconds )); then
    sleep_seconds="$remaining_seconds"
  fi

  if (( sleep_seconds > 0 )); then
    sleep "$sleep_seconds"
  fi
}

wait_for_service_ready() {
  local service="$1"
  local timeout_seconds="${2:-300}"
  local start_time now elapsed remaining_seconds status health_message container_id

  start_time="$(current_epoch_seconds)"

  while true; do
    status="$(compose_service_status "$service" 2>/dev/null || true)"
    case "$status" in
      healthy|running)
        return 0
        ;;
      exited|dead)
        die "Service '$service' crashed while waiting for readiness"
        ;;
      unhealthy)
        container_id="$(compose_service_container_id "$service" || true)"
        health_message=""
        if [[ -n "$container_id" ]]; then
          health_message="$(
            docker inspect \
              --format '{{if .State.Health}}{{range .State.Health.Log}}{{.Output}}{{printf "\n"}}{{end}}{{end}}' \
              "$container_id" 2>/dev/null \
              | tail -n 1 \
              | tr -d '\r'
          )"
        fi
        if [[ -n "$health_message" ]]; then
          die "Service '$service' became unhealthy while waiting for readiness: $health_message"
        fi
        die "Service '$service' became unhealthy while waiting for readiness"
        ;;
    esac

    now="$(current_epoch_seconds)"
    elapsed=$((now - start_time))
    remaining_seconds=$((timeout_seconds - elapsed))
    if (( remaining_seconds <= 0 )); then
      die "Timed out while waiting for service readiness '$service' (${timeout_seconds} sec.)"
    fi

    sleep_for_readiness_poll "$remaining_seconds"
  done
}

# Spend one shared readiness budget across sequential waits.
wait_for_service_ready_with_shared_timeout() {
  local timeout_budget_var_name="$1"
  local service="$2"
  local timeout_scope="${3:-operation}"
  local -n timeout_budget_ref="$timeout_budget_var_name"
  local wait_started wait_finished elapsed

  if (( timeout_budget_ref <= 0 )); then
    die "Shared readiness timeout for $timeout_scope was exhausted before service '$service'"
  fi

  wait_started="$(current_epoch_seconds)"
  wait_for_service_ready "$service" "$timeout_budget_ref"
  wait_finished="$(current_epoch_seconds)"
  elapsed=$((wait_finished - wait_started))
  timeout_budget_ref=$((timeout_budget_ref - elapsed))

  if (( timeout_budget_ref < 0 )); then
    timeout_budget_ref=0
  fi
}

wait_for_application_stack_with_shared_timeout() {
  local timeout_budget_var_name="$1"
  local timeout_scope="${2:-application stack}"

  wait_for_service_ready_with_shared_timeout "$timeout_budget_var_name" db "$timeout_scope"
  wait_for_service_ready_with_shared_timeout "$timeout_budget_var_name" espocrm "$timeout_scope"
  wait_for_service_ready_with_shared_timeout "$timeout_budget_var_name" espocrm-daemon "$timeout_scope"
  wait_for_service_ready_with_shared_timeout "$timeout_budget_var_name" espocrm-websocket "$timeout_scope"
}

wait_for_application_stack() {
  local timeout_seconds="${1:-300}"
  local timeout_scope="${2:-application stack}"
  # shellcheck disable=SC2034
  local timeout_budget="$timeout_seconds"

  wait_for_application_stack_with_shared_timeout timeout_budget "$timeout_scope"
}

http_probe() {
  local url="$1"

  if command_exists curl; then
    curl -fsSL --max-time 10 -o /dev/null "$url"
  elif command_exists wget; then
    wget -q -T 10 -O /dev/null "$url"
  else
    die "No HTTP probe tool found (curl or wget)"
  fi
}

is_tcp_port_busy() {
  local port="$1"

  if command_exists ss; then
    ss -ltn "( sport = :$port )" | tail -n +2 | grep -q .
  elif command_exists lsof; then
    lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1
  elif command_exists netstat; then
    netstat -ltn 2>/dev/null | awk 'NR > 2 { print $4 }' | grep -Eq "(^|:)$port$"
  else
    return 2
  fi
}
