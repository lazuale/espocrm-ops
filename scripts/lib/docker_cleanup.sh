#!/usr/bin/env bash
set -Eeuo pipefail

cleanup_duration_to_seconds() {
  local value="$1"
  local number suffix multiplier

  [[ "$value" =~ ^([0-9]+)([smhdw])$ ]] || return 1
  number="${BASH_REMATCH[1]}"
  suffix="${BASH_REMATCH[2]}"

  case "$suffix" in
    s) multiplier=1 ;;
    m) multiplier=60 ;;
    h) multiplier=3600 ;;
    d) multiplier=86400 ;;
    w) multiplier=604800 ;;
    *) return 1 ;;
  esac

  printf '%s\n' "$((number * multiplier))"
}

cleanup_validate_duration() {
  local name="$1"
  local value="$2"
  cleanup_duration_to_seconds "$value" >/dev/null || die "Parameter $name must use a format such as 30m, 12h, 7d, or 2w: $value"
}

cleanup_cutoff_epoch_for_duration() {
  local value="$1"
  local now seconds

  now="$(date +%s)"
  seconds="$(cleanup_duration_to_seconds "$value")"
  printf '%s\n' "$((now - seconds))"
}

cleanup_docker_time_to_epoch() {
  local value="$1"
  date -d "$value" +%s
}

cleanup_short_id() {
  local value="$1"
  value="${value#sha256:}"
  printf '%s\n' "${value:0:12}"
}

cleanup_setup_report_stream() {
  mkdir -p "$REPORT_DIR"
  cleanup_old_files "$REPORT_DIR" "$REPORT_RETENTION_DAYS" '*.txt'
  REPORT_FILE="$REPORT_DIR/docker-cleanup_$([[ $APPLY -eq 1 ]] && echo apply || echo plan)_${STAMP}.txt"

  if command_exists tee; then
    exec > >(tee "$REPORT_FILE") 2>&1
  fi
}

cleanup_require_docker() {
  command_exists docker || die "docker not found in PATH"
  docker info >/dev/null 2>&1 || die "Docker daemon is unavailable"
}

cleanup_build_referenced_image_index() {
  local container_ids=()
  local image_id

  mapfile -t container_ids < <(docker container ls -aq)
  [[ ${#container_ids[@]} -gt 0 ]] || return 0

  while IFS= read -r image_id; do
    [[ -n "$image_id" ]] || continue
    REFERENCED_IMAGE_IDS["$image_id"]=1
  done < <(docker inspect --format '{{.Image}}' "${container_ids[@]}" | sort -u)
}

cleanup_gather_stopped_container_candidates() {
  local cutoff_epoch container_ids=()
  local id name created status image created_epoch

  cutoff_epoch="$(cleanup_cutoff_epoch_for_duration "$CONTAINER_AGE")"
  mapfile -t container_ids < <(docker container ls -aq)
  [[ ${#container_ids[@]} -gt 0 ]] || return 0

  while IFS='|' read -r id name created status image; do
    [[ -n "$id" ]] || continue
    [[ "$status" == "running" ]] && continue
    created_epoch="$(cleanup_docker_time_to_epoch "$created" 2>/dev/null || true)"
    [[ "$created_epoch" =~ ^[0-9]+$ ]] || continue
    (( created_epoch <= cutoff_epoch )) || continue

    CONTAINER_IDS+=("$id")
    CONTAINER_LINES+=("$(cleanup_short_id "$id")  ${name#/}  status=$status  image=$image  created=$created")
  done < <(docker inspect --format '{{.Id}}|{{.Name}}|{{.Created}}|{{.State.Status}}|{{.Config.Image}}' "${container_ids[@]}")
}

cleanup_gather_image_candidates() {
  local image_ids=()
  local dangling_ids=()
  local cutoff_dangling cutoff_unused
  local id tag created created_epoch reason
  declare -A dangling_index=()
  declare -A seen=()

  cutoff_dangling="$(cleanup_cutoff_epoch_for_duration "$IMAGE_AGE")"
  cutoff_unused="$(cleanup_cutoff_epoch_for_duration "$UNUSED_IMAGE_AGE")"

  mapfile -t image_ids < <(docker image ls -qa --no-trunc | sort -u)
  [[ ${#image_ids[@]} -gt 0 ]] || return 0

  mapfile -t dangling_ids < <(docker image ls -q --no-trunc --filter dangling=true | sort -u)
  for id in "${dangling_ids[@]}"; do
    [[ -n "$id" ]] || continue
    dangling_index["$id"]=1
  done

  while IFS='|' read -r id tag created; do
    [[ -n "$id" ]] || continue
    created_epoch="$(cleanup_docker_time_to_epoch "$created" 2>/dev/null || true)"
    [[ "$created_epoch" =~ ^[0-9]+$ ]] || continue
    reason=""

    if [[ -n "${dangling_index[$id]+x}" ]] && (( created_epoch <= cutoff_dangling )); then
      reason="dangling"
    elif [[ $INCLUDE_UNUSED_IMAGES -eq 1 && -z "${REFERENCED_IMAGE_IDS[$id]+x}" ]] && (( created_epoch <= cutoff_unused )); then
      reason="unused"
    fi

    [[ -n "$reason" ]] || continue
    [[ -z "${seen[$id]+x}" ]] || continue

    IMAGE_IDS+=("$id")
    IMAGE_LINES+=("$(cleanup_short_id "$id")  $tag  reason=$reason  created=$created")
    seen["$id"]=1
  done < <(docker image inspect --format '{{.Id}}|{{if .RepoTags}}{{index .RepoTags 0}}{{else}}<none>:<none>{{end}}|{{.Created}}' "${image_ids[@]}")
}

cleanup_gather_network_candidates() {
  local cutoff_epoch network_ids=()
  local id name created container_count created_epoch

  cutoff_epoch="$(cleanup_cutoff_epoch_for_duration "$NETWORK_AGE")"
  mapfile -t network_ids < <(docker network ls -q)
  [[ ${#network_ids[@]} -gt 0 ]] || return 0

  while IFS='|' read -r id name created container_count; do
    [[ -n "$id" ]] || continue

    case "$name" in
      bridge|host|none)
        continue
        ;;
    esac

    [[ "$container_count" == "0" ]] || continue
    created_epoch="$(cleanup_docker_time_to_epoch "$created" 2>/dev/null || true)"
    [[ "$created_epoch" =~ ^[0-9]+$ ]] || continue
    (( created_epoch <= cutoff_epoch )) || continue

    NETWORK_IDS+=("$id")
    NETWORK_LINES+=("$(cleanup_short_id "$id")  $name  created=$created")
  done < <(docker network inspect --format '{{.Id}}|{{.Name}}|{{.Created}}|{{len .Containers}}' "${network_ids[@]}")
}

cleanup_print_candidate_block() {
  local title="$1"
  shift
  local lines=("$@")
  local line

  echo
  echo "== $title =="
  if [[ ${#lines[@]} -eq 0 ]]; then
    echo "Nothing found."
    return
  fi

  for line in "${lines[@]}"; do
    echo "$line"
  done
}

cleanup_remove_stopped_containers() {
  local id removed=0 failed=0

  [[ ${#CONTAINER_IDS[@]} -gt 0 ]] || return 0
  echo
  echo "Removing stopped containers..."

  for id in "${CONTAINER_IDS[@]}"; do
    if docker rm "$id" >/dev/null; then
      removed=$((removed + 1))
    else
      warn "Could not remove container: $id"
      failed=$((failed + 1))
    fi
  done

  note "Stopped containers: removed=$removed, failures=$failed"
  [[ $failed -eq 0 ]]
}

cleanup_remove_image_candidates() {
  local id removed=0 failed=0

  [[ ${#IMAGE_IDS[@]} -gt 0 ]] || return 0
  echo
  echo "Removing images..."

  for id in "${IMAGE_IDS[@]}"; do
    if docker image rm "$id" >/dev/null; then
      removed=$((removed + 1))
    else
      warn "Could not remove image: $id"
      failed=$((failed + 1))
    fi
  done

  note "Images: removed=$removed, failures=$failed"
  [[ $failed -eq 0 ]]
}

cleanup_remove_network_candidates() {
  local id removed=0 failed=0

  [[ ${#NETWORK_IDS[@]} -gt 0 ]] || return 0
  echo
  echo "Removing unused networks..."

  for id in "${NETWORK_IDS[@]}"; do
    if docker network rm "$id" >/dev/null; then
      removed=$((removed + 1))
    else
      warn "Could not remove network: $id"
      failed=$((failed + 1))
    fi
  done

  note "Networks: removed=$removed, failures=$failed"
  [[ $failed -eq 0 ]]
}

cleanup_prune_builder_cache() {
  [[ $SKIP_BUILD_CACHE -eq 1 ]] && return 0

  echo
  echo "Pruning build cache older than $BUILDER_AGE..."
  docker builder prune -f --filter "until=$BUILDER_AGE"
}
