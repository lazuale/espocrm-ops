#!/usr/bin/env bash
set -Eeuo pipefail

sha256_file() {
  local file="$1"
  [[ -f "$file" ]] || die "File for SHA-256 calculation not found: $file"

  if command_exists sha256sum; then
    sha256sum "$file" | awk '{print $1}'
  elif command_exists shasum; then
    shasum -a 256 "$file" | awk '{print $1}'
  elif command_exists openssl; then
    openssl dgst -sha256 "$file" | awk '{print $NF}'
  else
    die "No SHA-256 tool found (sha256sum, shasum, or openssl)"
  fi
}

write_sha256_sidecar() {
  local file="$1"
  local checksum="$2"
  local sidecar="${3:-$file.sha256}"

  printf '%s  %s\n' "$checksum" "$(basename "$file")" > "$sidecar"
}

read_sha256_sidecar() {
  local sidecar="$1"
  [[ -f "$sidecar" ]] || die "Checksum file not found: $sidecar"

  awk 'NR == 1 { print $1 }' "$sidecar"
}

verify_sha256_sidecar() {
  local file="$1"
  local sidecar="${2:-$file.sha256}"

  if [[ ! -f "$sidecar" ]]; then
    return 2
  fi

  local expected actual
  expected="$(read_sha256_sidecar "$sidecar")"
  actual="$(sha256_file "$file")"

  [[ "$actual" == "$expected" ]]
}

verify_sha256_required() {
  local file="$1"
  local sidecar="${2:-$file.sha256}"

  [[ -f "$sidecar" ]] || die "Checksum file is required for a destructive operation: $sidecar"

  if verify_sha256_sidecar "$file" "$sidecar"; then
    note "Checksum verified: $sidecar"
  else
    die "Checksum mismatch for file: $file"
  fi
}

backup_file_checksum_status() {
  local file="$1"
  local verify_checksum="${2:-0}"
  local sidecar

  if [[ ! -f "$file" ]]; then
    printf 'missing\n'
    return 0
  fi

  sidecar="${file}.sha256"
  if [[ ! -f "$sidecar" ]]; then
    printf 'sidecar_missing\n'
    return 0
  fi

  if [[ $verify_checksum -eq 0 ]]; then
    printf 'sidecar_present\n'
    return 0
  fi

  if verify_sha256_sidecar "$file" "$sidecar"; then
    printf 'verified\n'
  else
    printf 'mismatch\n'
  fi
}

file_size_bytes() {
  local file="$1"
  [[ -f "$file" ]] || die "File not found: $file"

  wc -c < "$file" | tr -d '[:space:]'
}

file_mtime_epoch() {
  local file="$1"
  [[ -f "$file" ]] || die "File not found: $file"

  if stat -c %Y "$file" >/dev/null 2>&1; then
    stat -c %Y "$file"
  elif stat -f %m "$file" >/dev/null 2>&1; then
    stat -f %m "$file"
  else
    die "Could not determine file modification time: $file"
  fi
}

file_age_hours() {
  local file="$1"
  local now mtime

  now="$(date +%s)"
  mtime="$(file_mtime_epoch "$file")"
  echo $(((now - mtime) / 3600))
}

latest_backup_file() {
  local directory="$1"
  local pattern="$2"

  [[ -d "$directory" ]] || return 1

  find "$directory" -maxdepth 1 -type f -name "$pattern" -printf '%T@ %p\n' \
    | sort -nr \
    | head -n 1 \
    | cut -d' ' -f2-
}

latest_verified_backup_file() {
  local directory="$1"
  local pattern="$2"
  local file

  [[ -d "$directory" ]] || return 1

  while IFS= read -r file; do
    [[ -n "$file" ]] || continue
    [[ -f "${file}.sha256" ]] || continue
    verify_sha256_sidecar "$file" "${file}.sha256" || continue
    printf '%s\n' "$file"
    return 0
  done < <(
    find "$directory" -maxdepth 1 -type f -name "$pattern" -printf '%T@ %p\n' \
      | sort -nr \
      | cut -d' ' -f2-
  )

  return 1
}

backup_group_from_db_file() {
  local file="$1"
  local base prefix stamp

  base="$(basename "$file")"
  if [[ "$base" =~ ^(.+)_([0-9]{4}-[0-9]{2}-[0-9]{2}_[0-9]{2}-[0-9]{2}-[0-9]{2})\.sql\.gz$ ]]; then
    prefix="${BASH_REMATCH[1]}"
    stamp="${BASH_REMATCH[2]}"
    printf '%s|%s\n' "$prefix" "$stamp"
    return 0
  fi

  return 1
}

backup_group_from_files_file() {
  local file="$1"
  local base prefix stamp

  base="$(basename "$file")"
  if [[ "$base" =~ ^(.+)_files_([0-9]{4}-[0-9]{2}-[0-9]{2}_[0-9]{2}-[0-9]{2}-[0-9]{2})\.tar\.gz$ ]]; then
    prefix="${BASH_REMATCH[1]}"
    stamp="${BASH_REMATCH[2]}"
    printf '%s|%s\n' "$prefix" "$stamp"
    return 0
  fi

  return 1
}

backup_group_from_manifest_file() {
  local file="$1"
  local base prefix stamp kind

  base="$(basename "$file")"
  if [[ "$base" =~ ^(.+)_([0-9]{4}-[0-9]{2}-[0-9]{2}_[0-9]{2}-[0-9]{2}-[0-9]{2})\.manifest\.(txt|json)$ ]]; then
    prefix="${BASH_REMATCH[1]}"
    stamp="${BASH_REMATCH[2]}"
    kind="${BASH_REMATCH[3]}"
    printf '%s|%s|%s\n' "$prefix" "$stamp" "$kind"
    return 0
  fi

  return 1
}

backup_manifest_txt_is_valid() {
  local file="$1"
  [[ -f "$file" ]] || return 1

  grep -q '^created_at=' "$file" \
    && grep -q '^contour=' "$file" \
    && grep -q '^compose_project=' "$file"
}

backup_manifest_json_is_valid() {
  local file="$1"
  [[ -f "$file" ]] || return 1

  grep -q '"version"' "$file" \
    && grep -q '"scope"' "$file" \
    && grep -q '"created_at"' "$file" \
    && grep -q '"artifacts"' "$file" \
    && grep -q '"checksums"' "$file" \
    && grep -q '"db_backup"' "$file" \
    && grep -q '"files_backup"' "$file"
}

backup_set_manifests_are_valid() {
  local manifest_txt="$1"
  local manifest_json="$2"

  backup_manifest_txt_is_valid "$manifest_txt" \
    && backup_manifest_json_is_valid "$manifest_json"
}

backup_set_paths() {
  local backup_root_abs="$1"
  local prefix="$2"
  local stamp="$3"

  printf '%s|%s|%s|%s\n' \
    "$backup_root_abs/db/${prefix}_${stamp}.sql.gz" \
    "$backup_root_abs/files/${prefix}_files_${stamp}.tar.gz" \
    "$backup_root_abs/manifests/${prefix}_${stamp}.manifest.txt" \
    "$backup_root_abs/manifests/${prefix}_${stamp}.manifest.json"
}

matching_files_backup_for_db() {
  local backup_root_abs="$1"
  local db_file="$2"
  local prefix stamp

  IFS='|' read -r prefix stamp < <(backup_group_from_db_file "$db_file") || return 1
  printf '%s\n' "$backup_root_abs/files/${prefix}_files_${stamp}.tar.gz"
}

matching_db_backup_for_files() {
  local backup_root_abs="$1"
  local files_file="$2"
  local prefix stamp

  IFS='|' read -r prefix stamp < <(backup_group_from_files_file "$files_file") || return 1
  printf '%s\n' "$backup_root_abs/db/${prefix}_${stamp}.sql.gz"
}

backup_pair_is_coherent() {
  local db_file="$1"
  local files_file="$2"
  local db_prefix db_stamp files_prefix files_stamp

  IFS='|' read -r db_prefix db_stamp < <(backup_group_from_db_file "$db_file") || return 1
  IFS='|' read -r files_prefix files_stamp < <(backup_group_from_files_file "$files_file") || return 1

  [[ "$db_prefix" == "$files_prefix" && "$db_stamp" == "$files_stamp" ]]
}

emit_backup_group_keys() {
  local backup_root_abs="$1"
  local selection_mode="${2:-any}"
  local file prefix stamp kind

  case "$selection_mode" in
    any|db)
      if [[ -d "$backup_root_abs/db" ]]; then
        while IFS= read -r file; do
          [[ -n "$file" ]] || continue
          if IFS='|' read -r prefix stamp < <(backup_group_from_db_file "$file"); then
            printf '%s|%s\n' "$prefix" "$stamp"
          fi
        done < <(find "$backup_root_abs/db" -maxdepth 1 -type f -name '*.sql.gz' | sort)
      fi
      ;;
  esac

  case "$selection_mode" in
    any|files)
      if [[ -d "$backup_root_abs/files" ]]; then
        while IFS= read -r file; do
          [[ -n "$file" ]] || continue
          if IFS='|' read -r prefix stamp < <(backup_group_from_files_file "$file"); then
            printf '%s|%s\n' "$prefix" "$stamp"
          fi
        done < <(find "$backup_root_abs/files" -maxdepth 1 -type f -name '*.tar.gz' | sort)
      fi
      ;;
  esac

  case "$selection_mode" in
    any|manifests)
      if [[ -d "$backup_root_abs/manifests" ]]; then
        while IFS= read -r file; do
          [[ -n "$file" ]] || continue
          if IFS='|' read -r prefix stamp kind < <(backup_group_from_manifest_file "$file"); then
            printf '%s|%s\n' "$prefix" "$stamp"
          fi
        done < <(find "$backup_root_abs/manifests" -maxdepth 1 -type f \( -name '*.manifest.txt' -o -name '*.manifest.json' \) | sort)
      fi
      ;;
  esac
}

latest_backup_group_key() {
  local backup_root_abs="$1"
  local selection_mode="${2:-any}"

  emit_backup_group_keys "$backup_root_abs" "$selection_mode" \
    | sort -u -t '|' -k2,2r -k1,1r \
    | head -n 1
}

latest_complete_backup_group_key() {
  local backup_root_abs="$1"
  local require_db="${2:-1}"
  local require_files="${3:-1}"
  local require_manifests="${4:-0}"
  local verify_checksum="${5:-0}"
  local validate_manifests="${6:-0}"
  local key prefix stamp db_file files_file manifest_txt manifest_json selection_mode="any"

  if [[ $require_db -eq 1 ]]; then
    selection_mode="db"
  elif [[ $require_files -eq 1 ]]; then
    selection_mode="files"
  elif [[ $require_manifests -eq 1 ]]; then
    selection_mode="manifests"
  fi

  while IFS= read -r key; do
    [[ -n "$key" ]] || continue
    IFS='|' read -r prefix stamp <<< "$key"
    IFS='|' read -r db_file files_file manifest_txt manifest_json < <(backup_set_paths "$backup_root_abs" "$prefix" "$stamp")

    if [[ $require_db -eq 1 && ! -f "$db_file" ]]; then
      continue
    fi

    if [[ $require_files -eq 1 && ! -f "$files_file" ]]; then
      continue
    fi

    if [[ $require_manifests -eq 1 ]]; then
      [[ -f "$manifest_txt" && -f "$manifest_json" ]] || continue
      if [[ $validate_manifests -eq 1 ]]; then
        backup_set_manifests_are_valid "$manifest_txt" "$manifest_json" || continue
      fi
    fi

    if [[ $verify_checksum -eq 1 ]]; then
      if [[ $require_db -eq 1 ]]; then
        [[ -f "${db_file}.sha256" ]] || continue
        verify_sha256_sidecar "$db_file" "${db_file}.sha256" || continue
      fi

      if [[ $require_files -eq 1 ]]; then
        [[ -f "${files_file}.sha256" ]] || continue
        verify_sha256_sidecar "$files_file" "${files_file}.sha256" || continue
      fi
    fi

    printf '%s\n' "$key"
    return 0
  done < <(emit_backup_group_keys "$backup_root_abs" "$selection_mode" | sort -u -t '|' -k2,2r -k1,1r)

  return 1
}

# Reject restore archives with unexpected roots or dangerous entry types.
validate_tar_archive_for_storage_restore() {
  local archive="$1"
  local expected_root="$2"
  local entry normalized top_level line entry_type saw_entries=0

  [[ -f "$archive" ]] || die "Archive for verification not found: $archive"
  [[ -n "$expected_root" ]] || die "Expected archive root directory is not set"

  while IFS= read -r entry; do
    [[ -n "$entry" ]] || continue
    saw_entries=1
    normalized="${entry#./}"
    normalized="${normalized%/}"

    [[ -n "$normalized" ]] || continue
    [[ "$normalized" != /* ]] || die "Archive contains an absolute path: $entry"
    [[ ! "$normalized" =~ (^|/)\.\.(/|$) ]] || die "Archive contains an unsafe path: $entry"

    top_level="${normalized%%/*}"
    [[ "$top_level" == "$expected_root" ]] || die "Archive contains a path outside directory '$expected_root': $entry"
  done < <(tar -tzf "$archive")

  [[ $saw_entries -eq 1 ]] || die "Archive is empty: $archive"

  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    entry_type="${line:0:1}"

    case "$entry_type" in
      -|d)
        ;;
      l|h|b|c|p|s)
        die "Archive contains an unsupported entry type '$entry_type'"
        ;;
      *)
        die "Archive contains an unknown entry type '$entry_type'"
        ;;
    esac
  done < <(tar -tvzf "$archive")
}

# Never let support-bundle capture mask the original failure path.
run_support_bundle_capture() {
  local script_dir="$1"
  local contour="$2"
  local env_file="$3"
  local bundle_path="$4"
  local success_message="$5"
  local failure_message="$6"

  set +e
  ENV_FILE="$env_file" run_repo_script "$script_dir/support-bundle.sh" "$contour" --output "$bundle_path"
  local bundle_exit=$?
  set -e

  if [[ $bundle_exit -eq 0 ]]; then
    warn "$success_message: $bundle_path"
  else
    warn "$failure_message"
  fi
}

cleanup_old_files() {
  local directory="$1"
  local retention_days="$2"
  shift 2

  [[ -d "$directory" ]] || return 0
  [[ $# -gt 0 ]] || return 0

  local find_args=("$directory" -type f '(')
  local first=1
  local pattern

  for pattern in "$@"; do
    if [[ $first -eq 0 ]]; then
      find_args+=(-o)
    fi
    find_args+=(-name "$pattern")
    first=0
  done

  find_args+=(')' -mtime +"$retention_days" -delete)
  find "${find_args[@]}"
}

directory_size_human() {
  local path="$1"
  local output=""

  if [[ ! -e "$path" ]]; then
    printf '0\n'
    return
  fi

  if command_exists du; then
    output="$(du -sh "$path" 2>/dev/null | awk 'NR == 1 { print $1 }' || true)"
    if [[ -n "$output" ]]; then
      printf '%s\n' "$output"
    else
      printf 'n/a\n'
    fi
  else
    printf 'n/a\n'
  fi
}
