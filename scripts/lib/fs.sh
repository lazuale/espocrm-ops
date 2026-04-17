#!/usr/bin/env bash
set -Eeuo pipefail

select_cleanup_helper_image() {
  local candidate
  local candidates=()

  if [[ -n "${ESPO_CLEANUP_IMAGE:-}" ]]; then
    candidates+=("$ESPO_CLEANUP_IMAGE")
  fi

  candidates+=("alpine:3.20" "busybox:1.36")

  if [[ -n "${MARIADB_TAG:-}" ]]; then
    candidates+=("mariadb:${MARIADB_TAG}")
  fi

  if [[ -n "${ESPOCRM_IMAGE:-}" ]]; then
    candidates+=("$ESPOCRM_IMAGE")
  fi

  for candidate in "${candidates[@]}"; do
    [[ -n "$candidate" ]] || continue
    if docker image inspect "$candidate" >/dev/null 2>&1; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done

  return 1
}

docker_remove_tree() {
  local target="$1"
  local parent base
  local cleanup_image

  parent="$(dirname "$target")"
  base="$(basename "$target")"

  command_exists docker || return 1
  [[ -d "$parent" ]] || return 1
  cleanup_image="$(select_cleanup_helper_image)" || return 1

  CLEANUP_BASENAME="$base" docker run --pull=never --rm \
    --entrypoint sh \
    -v "$parent:/cleanup-parent" \
    -e CLEANUP_BASENAME \
    "$cleanup_image" \
    -euc '
      target="/cleanup-parent/$CLEANUP_BASENAME"
      if [ -d "$target" ]; then
        find "$target" -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +
        rmdir "$target" 2>/dev/null || true
      fi
    ' \
    >/dev/null
}

remove_tree_failure_supports_docker_fallback() {
  local target="$1"
  local error_file="$2"

  [[ -d "$target" ]] || return 1
  [[ -f "$error_file" ]] || return 1

  if grep -Fq -e 'Permission denied' -e 'Operation not permitted' "$error_file"; then
    return 0
  fi

  if path_is_replaceable_directory "$target" && ! path_is_writable_or_creatable "$target"; then
    return 0
  fi

  return 1
}

# Use Docker fallback only for bind-mount ownership failures.
remove_tree_with_reasoned_fallback() {
  local target="$1"
  local error_file error_excerpt

  [[ -n "$target" ]] || return 1
  [[ -e "$target" ]] || return 0

  error_file="$(mktemp "${TMPDIR:-/tmp}/espo-rm.XXXXXX")"

  if LC_ALL=C rm -rf -- "$target" 2>"$error_file"; then
    rm -f -- "$error_file"
    return 0
  fi

  if [[ ! -e "$target" ]]; then
    rm -f -- "$error_file"
    return 0
  fi

  error_excerpt="$(head -n 1 "$error_file" 2>/dev/null || true)"

  if ! remove_tree_failure_supports_docker_fallback "$target" "$error_file"; then
    rm -f -- "$error_file"
    if [[ -n "$error_excerpt" ]]; then
      warn "Regular removal failed without signs of a permission or ownership scenario: $target ($error_excerpt)"
    else
      warn "Regular removal failed without signs of a permission or ownership scenario: $target"
    fi
    return 1
  fi

  rm -f -- "$error_file"
  warn "Regular removal hit a permission or ownership scenario, trying Docker fallback: $target"
  docker_remove_tree "$target" || return 1

  if [[ -d "$target" ]]; then
    rmdir "$target" 2>/dev/null || rm -rf -- "$target" 2>/dev/null || true
  fi

  if [[ -d "$target" ]]; then
    if find "$target" -mindepth 1 -print -quit | grep -q .; then
      return 1
    fi
    warn "Directory remained empty after Docker fallback, leaving it as-is: $target"
  fi

  return 0
}

docker_extract_tar_archive() {
  local archive="$1"
  local target_dir="$2"
  local cleanup_image archive_dir archive_base

  command_exists docker || return 1
  [[ -f "$archive" ]] || return 1

  mkdir -p "$target_dir"
  archive_dir="$(dirname "$archive")"
  archive_base="$(basename "$archive")"
  cleanup_image="$(select_cleanup_helper_image)" || return 1

  RESTORE_ARCHIVE_BASENAME="$archive_base" docker run --pull=never --rm \
    --entrypoint sh \
    -v "$archive_dir:/restore-archive:ro" \
    -v "$target_dir:/restore-target" \
    -e RESTORE_ARCHIVE_BASENAME \
    "$cleanup_image" \
    -euc 'tar -C /restore-target -xzf "/restore-archive/$RESTORE_ARCHIVE_BASENAME"' \
    >/dev/null
}

docker_create_tar_archive() {
  local source_dir="$1"
  local archive="$2"
  local source_parent source_base archive_dir archive_base cleanup_image

  command_exists docker || return 1
  [[ -d "$source_dir" ]] || return 1

  source_parent="$(dirname "$source_dir")"
  source_base="$(basename "$source_dir")"
  archive_dir="$(dirname "$archive")"
  archive_base="$(basename "$archive")"

  mkdir -p "$archive_dir"
  cleanup_image="$(select_cleanup_helper_image)" || return 1

  ARCHIVE_SOURCE_BASENAME="$source_base" ARCHIVE_OUTPUT_BASENAME="$archive_base" docker run --pull=never --rm \
    --entrypoint sh \
    -v "$source_parent:/archive-source:ro" \
    -v "$archive_dir:/archive-target" \
    -e ARCHIVE_SOURCE_BASENAME \
    -e ARCHIVE_OUTPUT_BASENAME \
    "$cleanup_image" \
    -euc 'tar -C /archive-source -czf "/archive-target/$ARCHIVE_OUTPUT_BASENAME" "$ARCHIVE_SOURCE_BASENAME"' \
    >/dev/null
}

create_tar_archive() {
  local source_dir="$1"
  local archive="$2"

  [[ -d "$source_dir" ]] || return 1
  mkdir -p "$(dirname "$archive")"

  if tar -C "$(dirname "$source_dir")" -czf "$archive" "$(basename "$source_dir")" >/dev/null 2>&1; then
    return 0
  fi

  warn "Local archiving failed, trying Docker fallback: $source_dir"
  docker_create_tar_archive "$source_dir" "$archive"
}

extract_tar_archive() {
  local archive="$1"
  local target_dir="$2"

  [[ -f "$archive" ]] || return 1
  mkdir -p "$target_dir"

  if tar -C "$target_dir" -xzf "$archive" >/dev/null 2>&1; then
    return 0
  fi

  warn "Local extraction failed, trying Docker fallback: $archive"
  docker_extract_tar_archive "$archive" "$target_dir"
}

# Read the runtime owner from the image instead of guessing host ids.
espocrm_runtime_owner_from_image() {
  local image="${1:-${ESPOCRM_IMAGE:-}}"
  local owner=""

  [[ -n "$image" ]] || return 1
  command_exists docker || return 1
  docker image inspect "$image" >/dev/null 2>&1 || return 1

  owner="$(
    docker run --pull=never --rm --user 0:0 --entrypoint sh \
      "$image" \
      -euc '
        for path in \
          /var/www/html/data \
          /var/www/html/custom \
          /var/www/html/client/custom \
          /var/www/html/upload \
          /var/www/html
        do
          if [ -e "$path" ]; then
            set -- $(ls -nd "$path")
            [ "$#" -ge 4 ] || exit 1
            printf "%s:%s\n" "$3" "$4"
            exit 0
          fi
        done

        exit 1
      '
  )" || return 1

  [[ "$owner" =~ ^[0-9]+:[0-9]+$ ]] || return 1
  printf '%s\n' "$owner"
}

docker_reconcile_espocrm_storage_permissions() {
  local target_dir="$1"
  local runtime_uid="$2"
  local runtime_gid="$3"
  local cleanup_image

  command_exists docker || return 1
  [[ -d "$target_dir" ]] || return 1
  [[ "$runtime_uid" =~ ^[0-9]+$ ]] || return 1
  [[ "$runtime_gid" =~ ^[0-9]+$ ]] || return 1

  cleanup_image="$(select_cleanup_helper_image)" || return 1

  ESPO_RUNTIME_UID="$runtime_uid" ESPO_RUNTIME_GID="$runtime_gid" docker run --pull=never --rm \
    --user 0:0 \
    --entrypoint sh \
    -v "$target_dir:/espo-storage" \
    -e ESPO_RUNTIME_UID \
    -e ESPO_RUNTIME_GID \
    "$cleanup_image" \
    -euc '
      storage="/espo-storage"

      chown -R "$ESPO_RUNTIME_UID:$ESPO_RUNTIME_GID" "$storage"
      find "$storage" -type d -exec chmod 0755 {} +

      for relative in data custom client/custom upload; do
        path="$storage/$relative"
        if [ -d "$path" ]; then
          find "$path" -type d -exec chmod 0775 {} +
          find "$path" -type f -exec chmod 0664 {} +
        fi
      done
    ' \
    >/dev/null
}

reconcile_espocrm_storage_permissions() {
  local target_dir="$1"
  local runtime_owner runtime_uid runtime_gid

  [[ -d "$target_dir" ]] || return 1

  runtime_owner="$(espocrm_runtime_owner_from_image "${ESPOCRM_IMAGE:-}")" || return 1
  IFS=':' read -r runtime_uid runtime_gid <<<"$runtime_owner"
  [[ -n "$runtime_uid" && -n "$runtime_gid" ]] || return 1

  docker_reconcile_espocrm_storage_permissions "$target_dir" "$runtime_uid" "$runtime_gid"
}

docker_move_tree_within_dir() {
  local workspace="$1"
  local source_rel="$2"
  local target_rel="$3"
  local cleanup_image

  command_exists docker || return 1
  [[ -d "$workspace" ]] || return 1
  [[ -n "$source_rel" && "$source_rel" != /* ]] || return 1
  [[ -n "$target_rel" && "$target_rel" != /* ]] || return 1
  cleanup_image="$(select_cleanup_helper_image)" || return 1

  MOVE_SOURCE_REL="$source_rel" MOVE_TARGET_REL="$target_rel" docker run --pull=never --rm \
    --entrypoint sh \
    -v "$workspace:/move-workspace" \
    -e MOVE_SOURCE_REL \
    -e MOVE_TARGET_REL \
    "$cleanup_image" \
    -euc '
      source="/move-workspace/$MOVE_SOURCE_REL"
      target="/move-workspace/$MOVE_TARGET_REL"
      target_parent="$(dirname "$target")"

      [ -e "$source" ] || exit 1
      mkdir -p "$target_parent"
      rm -rf -- "$target"
      mv -- "$source" "$target"
    ' \
    >/dev/null
}

move_tree_within_dir() {
  local workspace="$1"
  local source_rel="$2"
  local target_rel="$3"
  local source_path target_path target_parent

  [[ -d "$workspace" ]] || return 1
  [[ -n "$source_rel" && "$source_rel" != /* ]] || return 1
  [[ -n "$target_rel" && "$target_rel" != /* ]] || return 1

  source_path="$workspace/$source_rel"
  target_path="$workspace/$target_rel"
  target_parent="$(dirname "$target_path")"

  [[ -e "$source_path" ]] || return 1
  mkdir -p "$target_parent"

  if mv -- "$source_path" "$target_path" 2>/dev/null; then
    return 0
  fi

  warn "Local move failed, trying Docker fallback: $source_path -> $target_path"
  docker_move_tree_within_dir "$workspace" "$source_rel" "$target_rel"
}

# Quarantine the old tree if direct removal is unsafe.
replace_tree_within_dir() {
  local workspace="$1"
  local source_rel="$2"
  local target_rel="$3"
  local source_path target_path target_parent target_base
  local quarantined_rel="" quarantined_path=""
  local quarantine_stamp

  [[ -d "$workspace" ]] || return 1
  [[ -n "$source_rel" && "$source_rel" != /* ]] || return 1
  [[ -n "$target_rel" && "$target_rel" != /* ]] || return 1

  source_path="$workspace/$source_rel"
  target_path="$workspace/$target_rel"
  target_parent="$(dirname "$target_path")"
  target_base="$(basename "$target_rel")"

  [[ -e "$source_path" ]] || return 1
  mkdir -p "$target_parent"

  if [[ -e "$target_path" ]]; then
    if ! rm -rf -- "$target_path" 2>/dev/null; then
      quarantine_stamp="$(date +%s).$$"
      quarantined_rel=".quarantine.${target_base}.${quarantine_stamp}"
      quarantined_path="$workspace/$quarantined_rel"

      if move_tree_within_dir "$workspace" "$target_rel" "$quarantined_rel"; then
        warn "Old directory temporarily moved to quarantine: $quarantined_path"
      else
        warn "Could not move the old directory to quarantine, trying safe removal with a reasoned fallback: $target_path"
        remove_tree_with_reasoned_fallback "$target_path" || return 1
        if [[ -e "$target_path" ]]; then
          return 1
        fi
      fi
    fi
  fi

  if ! move_tree_within_dir "$workspace" "$source_rel" "$target_rel"; then
    if [[ -n "$quarantined_rel" && -e "$quarantined_path" && ! -e "$target_path" ]]; then
      warn "Replacement failed, trying to restore the original directory from quarantine"
      move_tree_within_dir "$workspace" "$quarantined_rel" "$target_rel" >/dev/null 2>&1 || true
    fi
    return 1
  fi

  if [[ -n "$quarantined_rel" && -e "$quarantined_path" ]]; then
    if remove_tree_with_reasoned_fallback "$quarantined_path"; then
      return 0
    fi

    warn "Could not remove the old directory after replacement, leaving quarantine in place: $quarantined_path"
  fi
}

path_is_within() {
  local path="$1"
  local base="$2"
  [[ "$path" == "$base" || "$path" == "$base"/* ]]
}

file_mode_octal() {
  local path="$1"
  [[ -e "$path" ]] || die "Path not found: $path"

  if stat -c %a "$path" >/dev/null 2>&1; then
    stat -c %a "$path"
  elif stat -f %Lp "$path" >/dev/null 2>&1; then
    stat -f %Lp "$path"
  else
    die "Could not determine permissions for path: $path"
  fi
}

nearest_existing_parent() {
  local path="$1"

  while [[ ! -e "$path" && "$path" != "/" ]]; do
    path="$(dirname "$path")"
  done

  printf '%s\n' "$path"
}

path_is_writable_or_creatable() {
  local path="$1"
  local parent

  if [[ -e "$path" ]]; then
    [[ -w "$path" ]]
    return
  fi

  parent="$(nearest_existing_parent "$path")"
  [[ -d "$parent" && -w "$parent" ]]
}

path_is_replaceable_directory() {
  local path="$1"
  local parent

  [[ -d "$path" && -r "$path" && -x "$path" ]] || return 1
  parent="$(dirname "$path")"
  [[ -d "$parent" && -w "$parent" && -x "$parent" ]]
}

# Refuse to remove anything outside the allowed base.
safe_remove_tree() {
  local target="$1"
  local allowed_base="${2:-$ROOT_DIR}"
  [[ -n "$target" ]] || die "No path was provided for removal"
  [[ -d "$target" ]] || return 0

  local resolved resolved_allowed_base
  resolved="$(cd "$target" && pwd)"
  resolved_allowed_base="$(cd "$allowed_base" && pwd)"

  [[ -n "$resolved" ]] || die "Could not resolve the directory path for removal: $target"
  [[ -n "$resolved_allowed_base" ]] || die "Could not resolve the allowed removal base: $allowed_base"
  path_is_within "$resolved" "$resolved_allowed_base" || die "Refusing to remove a path outside the allowed base: $resolved"
  [[ "$resolved" != "$resolved_allowed_base" || "$resolved_allowed_base" != "/" ]] || die "Refusing to remove the filesystem root"
  [[ "$resolved" != "$ROOT_DIR" ]] || die "Refusing to remove the repository root"

  remove_tree_with_reasoned_fallback "$resolved" || die "Could not remove directory: $resolved"
}
