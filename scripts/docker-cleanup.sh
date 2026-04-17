#!/usr/bin/env bash
set -Eeuo pipefail

# Safe Docker host cleanup for old stopped containers,
# dangling images, unused networks, and build cache.
# By default it runs only in plan mode and removes nothing.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/locks.sh"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/artifacts.sh"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/docker_cleanup.sh"

usage() {
  cat <<'EOF'
Usage: ./scripts/docker-cleanup.sh [--apply] [--report-dir PATH] [--include-unused-images] [--skip-build-cache] [--container-age AGE] [--image-age AGE] [--unused-image-age AGE] [--network-age AGE] [--builder-age AGE]

Examples:
  ./scripts/docker-cleanup.sh
  ./scripts/docker-cleanup.sh --report-dir /opt/espocrm-data/backups/host/reports
  ./scripts/docker-cleanup.sh --apply
  ./scripts/docker-cleanup.sh --apply --include-unused-images
  ./scripts/docker-cleanup.sh --apply --container-age 168h --image-age 168h --network-age 168h --builder-age 168h

Parameters:
  --apply                  Actually remove the found candidates. Without this flag, only a dry run is performed.
  --report-dir PATH        Report directory. It can also be set via DOCKER_CLEANUP_REPORT_DIR.
  --include-unused-images  Also delete old unused tagged images, not just dangling ones.
  --skip-build-cache       Do not prune the build cache.
  --container-age AGE      Age threshold for stopped containers to be treated as garbage. Default: 168h.
  --image-age AGE          Age threshold for dangling images. Default: 168h.
  --unused-image-age AGE   Age threshold for tagged images without containers. Default: 336h.
  --network-age AGE        Age threshold for unused user-defined networks. Default: 168h.
  --builder-age AGE        Age threshold for build cache used by docker builder prune. Default: 168h.

AGE format:
  Supported values look like 30m, 12h, 7d, or 2w.

Important:
  This script intentionally does not prune volumes to avoid touching unrelated Docker data volumes.
  If the report directory is not set explicitly, the user XDG state directory
  or ~/.local/state/espocrm-toolkit/docker-cleanup/reports is used.
EOF
}

resolve_cleanup_report_dir() {
  local requested_path="${REPORT_DIR_ARG:-${DOCKER_CLEANUP_REPORT_DIR:-}}"

  if [[ -n "$requested_path" ]]; then
    REPORT_DIR="$(caller_path "$requested_path")"
    return 0
  fi

  if [[ -n "${XDG_STATE_HOME:-}" ]]; then
    REPORT_DIR="$XDG_STATE_HOME/espocrm-toolkit/docker-cleanup/reports"
    return 0
  fi

  if [[ -n "${HOME:-}" ]]; then
    REPORT_DIR="$HOME/.local/state/espocrm-toolkit/docker-cleanup/reports"
    return 0
  fi

  REPORT_DIR="${TMPDIR:-/tmp}/espocrm-toolkit/docker-cleanup/reports"
  warn "Could not determine HOME/XDG_STATE_HOME, using a temporary report directory: $REPORT_DIR"
}

APPLY=0
REPORT_DIR_ARG=""
INCLUDE_UNUSED_IMAGES=0
SKIP_BUILD_CACHE=0
CONTAINER_AGE="168h"
IMAGE_AGE="168h"
UNUSED_IMAGE_AGE="336h"
NETWORK_AGE="168h"
BUILDER_AGE="168h"
# Shared cleanup state: these variables are consumed by `scripts/lib/docker_cleanup.sh`.
# shellcheck disable=SC2034
STAMP=""
# shellcheck disable=SC2034
REPORT_DIR=""
# shellcheck disable=SC2034
REPORT_RETENTION_DAYS=30
REPORT_FILE=""
declare -a CONTAINER_IDS=()
declare -a CONTAINER_LINES=()
declare -a IMAGE_IDS=()
declare -a IMAGE_LINES=()
declare -a NETWORK_IDS=()
declare -a NETWORK_LINES=()
# shellcheck disable=SC2034
declare -A REFERENCED_IMAGE_IDS=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --apply)
      APPLY=1
      shift
      ;;
    --report-dir)
      [[ $# -ge 2 ]] || die "--report-dir must be followed by a path"
      REPORT_DIR_ARG="$2"
      shift 2
      ;;
    --include-unused-images)
      INCLUDE_UNUSED_IMAGES=1
      shift
      ;;
    --skip-build-cache)
      SKIP_BUILD_CACHE=1
      shift
      ;;
    --container-age)
      [[ $# -ge 2 ]] || die "--container-age must be followed by a value"
      CONTAINER_AGE="$2"
      shift 2
      ;;
    --image-age)
      [[ $# -ge 2 ]] || die "--image-age must be followed by a value"
      IMAGE_AGE="$2"
      shift 2
      ;;
    --unused-image-age)
      [[ $# -ge 2 ]] || die "--unused-image-age must be followed by a value"
      UNUSED_IMAGE_AGE="$2"
      shift 2
      ;;
    --network-age)
      [[ $# -ge 2 ]] || die "--network-age must be followed by a value"
      NETWORK_AGE="$2"
      shift 2
      ;;
    --builder-age)
      [[ $# -ge 2 ]] || die "--builder-age must be followed by a value"
      BUILDER_AGE="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      die "Unknown argument: $1"
      ;;
  esac
done

cleanup_validate_duration --container-age "$CONTAINER_AGE"
cleanup_validate_duration --image-age "$IMAGE_AGE"
cleanup_validate_duration --unused-image-age "$UNUSED_IMAGE_AGE"
cleanup_validate_duration --network-age "$NETWORK_AGE"
cleanup_validate_duration --builder-age "$BUILDER_AGE"
resolve_cleanup_report_dir

# shellcheck disable=SC2034
STAMP="$(next_unique_stamp "$REPORT_DIR/docker-cleanup_$([[ $APPLY -eq 1 ]] && echo apply || echo plan)__STAMP__.txt")"
acquire_operation_lock docker-cleanup
cleanup_setup_report_stream
cleanup_require_docker

echo "Mode: $([[ $APPLY -eq 1 ]] && echo apply || echo plan)"
echo "Report: $REPORT_FILE"
echo "Volume cleanup: intentionally disabled"
echo "Container threshold: $CONTAINER_AGE"
echo "Dangling image threshold: $IMAGE_AGE"
echo "Unused tagged-image threshold: $UNUSED_IMAGE_AGE"
echo "Network threshold: $NETWORK_AGE"
echo "Build-cache threshold: $BUILDER_AGE"
echo "Delete old unused tagged images: $([[ $INCLUDE_UNUSED_IMAGES -eq 1 ]] && echo yes || echo no)"
echo "Prune build cache: $([[ $SKIP_BUILD_CACHE -eq 1 ]] && echo no || echo yes)"
echo "Toolkit-operation serialization is enforced by the shared operation lock"

echo
echo "== Docker state before cleanup =="
docker system df || true

cleanup_build_referenced_image_index
cleanup_gather_stopped_container_candidates
cleanup_gather_image_candidates
cleanup_gather_network_candidates

cleanup_print_candidate_block "Stopped containers to remove (${#CONTAINER_IDS[@]})" "${CONTAINER_LINES[@]}"
cleanup_print_candidate_block "Images to remove (${#IMAGE_IDS[@]})" "${IMAGE_LINES[@]}"
cleanup_print_candidate_block "User-defined networks to remove (${#NETWORK_IDS[@]})" "${NETWORK_LINES[@]}"

echo
echo "== Build cache =="
if [[ $SKIP_BUILD_CACHE -eq 1 ]]; then
  echo "Build-cache pruning is disabled by --skip-build-cache."
else
  echo "Docker CLI does not provide an exact dry run for build-cache cleanup."
  echo "In apply mode the following command will run: docker builder prune -f --filter until=$BUILDER_AGE"
fi

if [[ $APPLY -eq 0 ]]; then
  echo
  echo "Dry run completed. Re-run with --apply to perform the cleanup."
  exit 0
fi

FAILURES=0
cleanup_remove_stopped_containers || FAILURES=$((FAILURES + 1))
cleanup_remove_image_candidates || FAILURES=$((FAILURES + 1))
cleanup_remove_network_candidates || FAILURES=$((FAILURES + 1))
cleanup_prune_builder_cache || FAILURES=$((FAILURES + 1))

echo
echo "== Docker state after cleanup =="
docker system df || true

if [[ $FAILURES -ne 0 ]]; then
  die "Cleanup finished with failures. Details were saved in the report: $REPORT_FILE"
fi

echo
echo "Cleanup completed successfully."
