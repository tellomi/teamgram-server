#!/usr/bin/env bash
# Shared helpers for tellomi-db-backup.sh and tellomi-backup-verify.sh.
# Sourced, not executed. Does not print secrets.
# shellcheck shell=bash

# LOG_TAG is set by the caller (tellomi-db-backup / tellomi-backup-verify).

backup_log() {
  echo "$*" >&2
  logger -t "${LOG_TAG:-tellomi-backup}" -- "$*" 2>/dev/null || true
}

backup_warn() {
  echo "================================================================================" >&2
  echo "WARNING: $*" >&2
  echo "================================================================================" >&2
  logger -t "${LOG_TAG:-tellomi-backup}" -p user.warning -- "$*" 2>/dev/null || true
}

backup_die() {
  echo "ERROR: $*" >&2
  logger -t "${LOG_TAG:-tellomi-backup}" -p user.err -- "$*" 2>/dev/null || true
  exit 1
}

# backup_set_rclone_flags fills RCLONE_S3_FLAGS.
#
# rclone 1.60.1 against Cloudflare R2 (docs/ops/BACKUP.md):
#   --s3-no-check-bucket   the first call would be CreateBucket, which R2
#                          answers with 501 NotImplemented. Object-scoped
#                          tokens cannot create buckets either.
#   --s3-upload-cutoff 5G  keep dumps under 5GiB on one PutObject. The
#                          multipart path in rclone 1.60.1 has returned 501
#                          for headers R2 does not implement.
#   --s3-acl private       R2 returns 501 for x-amz-acl other than private.
backup_set_rclone_flags() {
  local cutoff="${TELLOMI_RCLONE_UPLOAD_CUTOFF:-5G}"
  if [[ ! "$cutoff" =~ ^[0-9]+[KMGT]?$ ]]; then
    backup_die "TELLOMI_RCLONE_UPLOAD_CUTOFF must look like 5G or 200M"
  fi
  # --s3-use-presigned-request: 1.60.1's PutObject uses SetStreamingBody, so the
  # first request is chunked. R2 answers 501 (aws-chunked is not implemented)
  # and the SDK retries. A presigned PUT sets Content-Length and does not chunk.
  # --s3-no-head: skip the HEAD after PUT. The sha256 sidecar is the check.
  RCLONE_S3_FLAGS=(
    --s3-no-check-bucket
    --s3-upload-cutoff "$cutoff"
    --s3-acl private
    --s3-use-presigned-request
    --s3-no-head
  )
}

# backup_rclone runs rclone with the R2 flags appended. Credentials come from
# the environment (RCLONE_CONFIG_R2_*), never from argv.
backup_rclone() {
  backup_set_rclone_flags
  rclone "$@" "${RCLONE_S3_FLAGS[@]}"
}

# backup_read_env_file reads KEY=value lines. It does not execute the file.
backup_read_env_file() {
  local file="$1" line key val mode
  [[ -f "$file" ]] || backup_die "missing env file ${file}"
  mode="$(stat -c '%a' "$file")"
  if [[ ! "$mode" =~ ^[0-9]+$ ]]; then
    backup_die "could not read mode of ${file}"
  fi
  if [[ "${mode: -1}" != "0" ]]; then
    backup_die "${file} is world-accessible (mode ${mode}); chmod 600"
  fi
  if [[ "${mode: -2:1}" != "0" ]]; then
    backup_warn "${file} is group-readable (mode ${mode}); chmod 600"
  fi
  while IFS= read -r line || [[ -n "$line" ]]; do
    case "$line" in
      ''|\#*) continue ;;
    esac
    if [[ ! "$line" =~ ^[A-Za-z_][A-Za-z0-9_]*= ]]; then
      backup_die "${file} has a line that is not KEY=value"
    fi
    key="${line%%=*}"
    val="${line#*=}"
    if [[ "$val" == \"*\" && "$val" == *\" ]]; then
      val="${val:1:${#val}-2}"
    elif [[ "$val" == \'*\' && "$val" == *\' ]]; then
      val="${val:1:${#val}-2}"
    fi
    printf -v "$key" '%s' "$val"
    export "${key?}"
  done <"$file"
}

# backup_container_env prints one env value from a container, or empty.
backup_container_env() {
  local container="$1" key="$2" val rc
  set +e
  val="$(docker exec "$container" printenv "$key" 2>/dev/null)"
  rc=$?
  set -e
  if [[ "$rc" -ne 0 ]]; then
    printf ''
    return 0
  fi
  printf '%s' "$val"
}

# backup_load_r2 exports RCLONE_CONFIG_R2_* .
# Dedicated file: TELLOMI_BACKUP_R2_ENV (default /opt/tellomi/secrets/db-backup-r2.env).
# Missing file: loud warning and the teamgram media key, unless
# TELLOMI_BACKUP_REQUIRE_DEDICATED_KEY=1, which fails instead.
# Sets CRED_SOURCE to dedicated or media-fallback.
backup_load_r2() {
  local secrets id secret endpoint
  secrets="${TELLOMI_BACKUP_R2_ENV:-/opt/tellomi/secrets/db-backup-r2.env}"
  CRED_SOURCE=""
  if [[ -f "$secrets" ]]; then
    backup_read_env_file "$secrets"
    CRED_SOURCE="dedicated"
  elif [[ "${TELLOMI_BACKUP_REQUIRE_DEDICATED_KEY:-}" == "1" ]]; then
    backup_die "TELLOMI_BACKUP_REQUIRE_DEDICATED_KEY=1 but ${secrets} does not exist. Refusing the media-key fallback."
  else
    backup_warn "${secrets} does not exist. Using the teamgram media key TELLOMI_S3_ACCESS_KEY_ID / TELLOMI_S3_SECRET_ACCESS_KEY from container ${TELLOMI_TEAMGRAM_CONTAINER:-tellomi-teamgram-1}. That key can read media. Write a token scoped only to bucket tellomi-prod-db-backups, then set TELLOMI_BACKUP_REQUIRE_DEDICATED_KEY=1 so this fallback fails closed."
    CRED_SOURCE="media-fallback"
    id="$(backup_container_env "${TELLOMI_TEAMGRAM_CONTAINER:-tellomi-teamgram-1}" TELLOMI_S3_ACCESS_KEY_ID)"
    secret="$(backup_container_env "${TELLOMI_TEAMGRAM_CONTAINER:-tellomi-teamgram-1}" TELLOMI_S3_SECRET_ACCESS_KEY)"
    endpoint="$(backup_container_env "${TELLOMI_TEAMGRAM_CONTAINER:-tellomi-teamgram-1}" TELLOMI_S3_ENDPOINT)"
    if [[ -z "$id" || -z "$secret" ]]; then
      backup_die "media-key fallback: TELLOMI_S3_ACCESS_KEY_ID or TELLOMI_S3_SECRET_ACCESS_KEY is empty"
    fi
    export RCLONE_CONFIG_R2_ACCESS_KEY_ID="$id"
    export RCLONE_CONFIG_R2_SECRET_ACCESS_KEY="$secret"
    if [[ -n "$endpoint" ]]; then
      case "$endpoint" in
        http://*|https://*) ;;
        *) endpoint="https://${endpoint}" ;;
      esac
      export RCLONE_CONFIG_R2_ENDPOINT="$endpoint"
    fi
    unset id secret endpoint
  fi
  export RCLONE_CONFIG_R2_TYPE="${RCLONE_CONFIG_R2_TYPE:-s3}"
  export RCLONE_CONFIG_R2_PROVIDER="${RCLONE_CONFIG_R2_PROVIDER:-Cloudflare}"
  export RCLONE_CONFIG_R2_REGION="${RCLONE_CONFIG_R2_REGION:-auto}"
  export RCLONE_CONFIG_R2_ACL="${RCLONE_CONFIG_R2_ACL:-private}"
  export RCLONE_CONFIG_R2_NO_CHECK_BUCKET="${RCLONE_CONFIG_R2_NO_CHECK_BUCKET:-true}"
  if [[ -z "${RCLONE_CONFIG_R2_ACCESS_KEY_ID:-}" || -z "${RCLONE_CONFIG_R2_SECRET_ACCESS_KEY:-}" ]]; then
    backup_die "R2 access key or secret is empty after loading credentials (${CRED_SOURCE})"
  fi
  if [[ "${RCLONE_CONFIG_R2_TYPE}" != "s3" ]]; then
    backup_die "RCLONE_CONFIG_R2_TYPE must be s3"
  fi
}

# backup_age_header_ok checks the textual age header. It does not decrypt.
backup_age_header_ok() {
  local file="$1" line n=0 magic=0 recip=0 done_hdr=0
  [[ -f "$file" ]] || return 1
  while IFS= read -r line; do
    line="${line%$'\r'}"
    n=$((n + 1))
    if (( n > 32 )); then
      return 1
    fi
    if (( n == 1 )); then
      [[ "$line" == "age-encryption.org/v1" ]] || return 1
      magic=1
      continue
    fi
    # age ends the header with "---" or, in real files, "--- <base64 MAC>".
    if [[ "$line" == "---" || "$line" == "--- "* ]]; then
      done_hdr=1
      break
    fi
    if [[ "$line" == "->"* ]]; then
      recip=1
    fi
  done <"$file"
  [[ "$magic" -eq 1 && "$recip" -eq 1 && "$done_hdr" -eq 1 ]]
}

# backup_write_sha256 writes "HASH  basename" for sha256sum -c and prints the hash.
backup_write_sha256() {
  local file="$1" base sum
  base="$(basename "$file")"
  sum="$(sha256sum "$file" | awk '{print $1}')"
  if [[ ! "$sum" =~ ^[0-9a-f]{64}$ ]]; then
    backup_die "sha256sum failed for ${file}"
  fi
  printf '%s  %s\n' "$sum" "$base" >"${file}.sha256"
  chmod 600 "${file}.sha256"
  printf '%s' "$sum"
}

backup_git_sha() {
  local dir sha
  dir="${TELLOMI_DEPLOY_DIR:-/opt/tellomi/teamgram-server}"
  if [[ ! -e "$dir/.git" ]]; then
    printf 'unknown'
    return 0
  fi
  sha="$(git -C "$dir" rev-parse HEAD 2>/dev/null || true)"
  if [[ ! "$sha" =~ ^[0-9a-f]{40}$ ]]; then
    printf 'unknown'
    return 0
  fi
  printf '%s' "$sha"
}

# backup_dir_allowed refuses to prune the filesystem root or a short path.
backup_dir_allowed() {
  local root="$1"
  case "$root" in
    ""|/|/opt|/opt/tellomi|/usr|/usr/local|/var|/tmp|/home) return 1 ;;
  esac
  if [[ "${TELLOMI_BACKUP_ALLOW_DIR:-}" == "1" ]]; then
    [[ "$root" == /* ]] || return 1
    return 0
  fi
  case "$root" in
    /opt/tellomi/backups|/opt/tellomi/backups/*) return 0 ;;
  esac
  return 1
}

# backup_prune_local deletes files under root whose mtime is older than days.
# DRY=1 prints the paths and deletes nothing. Never calls rclone.
backup_prune_local() {
  local root="$1" days="$2" now cutoff ts path
  [[ "$days" =~ ^[0-9]+$ ]] || backup_die "retain days must be an integer"
  if ! backup_dir_allowed "$root"; then
    backup_die "refusing to prune ${root}"
  fi
  [[ -d "$root" ]] || return 0
  now="$(date +%s)"
  cutoff=$((now - days * 86400))
  while IFS= read -r ts && IFS= read -r path; do
    [[ -n "${path}" ]] || continue
    ts="${ts%%.*}"
    [[ "$ts" =~ ^[0-9]+$ ]] || continue
    if (( ts < cutoff )); then
      if [[ "${DRY:-0}" -eq 1 ]]; then
        backup_log "dry-run: would delete ${path}"
      else
        rm -f -- "$path"
        backup_log "deleted local ${path}"
      fi
    fi
  done < <(find "$root" -type f -printf '%T@\n%p\n')
  if [[ "${DRY:-0}" -eq 0 ]]; then
    find "$root" -mindepth 1 -type d -empty -delete
  fi
}

# backup_manifest_add records one ciphertext object. Fields are constrained so
# the JSON writer does not need an encoder.
backup_manifest_add() {
  local prefix="$1" name="$2" sum="$3" size="$4"
  [[ "$prefix" =~ ^(mysql|etcd|redis)$ ]] || backup_die "bad manifest prefix"
  [[ "$name" =~ ^[A-Za-z0-9._-]+$ ]] || backup_die "bad manifest name"
  [[ "$sum" =~ ^[0-9a-f]{64}$ ]] || backup_die "bad manifest sha256"
  [[ "$size" =~ ^[0-9]+$ ]] || backup_die "bad manifest size"
  MANIFEST_ROWS+=("${prefix} ${name} ${sum} ${size}")
}

# backup_write_manifest writes manifest JSON to path.
# Args: path started finished git_sha retain credentials
backup_write_manifest() {
  local path="$1" started="$2" finished="$3" sha="$4" retain="$5" creds="$6"
  local row prefix name sum size i=0 n
  [[ "$started" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$ ]] || backup_die "bad started_at"
  [[ "$finished" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$ ]] || backup_die "bad finished_at"
  [[ "$sha" =~ ^([0-9a-f]{40}|unknown)$ ]] || backup_die "bad git sha"
  [[ "$retain" =~ ^[0-9]+$ ]] || backup_die "bad retain days"
  [[ "$creds" == "dedicated" || "$creds" == "media-fallback" ]] || backup_die "bad credentials label"
  n="${#MANIFEST_ROWS[@]}"
  {
    printf '{\n'
    printf '  "schema": 1,\n'
    printf '  "started_at": "%s",\n' "$started"
    printf '  "finished_at": "%s",\n' "$finished"
    printf '  "git_sha": "%s",\n' "$sha"
    printf '  "retain_days": %s,\n' "$retain"
    printf '  "credentials": "%s",\n' "$creds"
    printf '  "files": [\n'
    for row in "${MANIFEST_ROWS[@]}"; do
      # shellcheck disable=SC2086
      read -r prefix name sum size <<<"$row"
      i=$((i + 1))
      printf '    {"prefix": "%s", "name": "%s", "sha256": "%s", "size": %s}' \
        "$prefix" "$name" "$sum" "$size"
      if (( i < n )); then
        printf ',\n'
      else
        printf '\n'
      fi
    done
    printf '  ]\n}\n'
  } >"$path"
  chmod 600 "$path"
}
