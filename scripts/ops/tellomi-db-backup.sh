#!/usr/bin/env bash
# Daily MySQL, etcd, and Redis backups (D-34, D-53, M7).
#
#   tellomi-db-backup.sh
#   tellomi-db-backup.sh --dry-run
#   tellomi-db-backup.sh --self-test
#
# Cron: scripts/ops/tellomi-db-backup.cron at 03:15.
# Install and R2 setup: docs/ops/BACKUP.md.
#
# MySQL: mysqldump of database teamgram inside the mysql container, gzip, age.
# etcd: etcdctl snapshot save into the /etcd-data mount, delete it on the host, gzip, age.
# The etcd image has no shell and no rm. mysql has sh and mysqldump. redis has redis-cli.
# Redis: BGSAVE, wait until LASTSAVE changes, then a read-only copy of
# dump.rdb and appendonlydir. No FLUSH, CONFIG, DEBUG, SHUTDOWN, or SAVE.
# Ciphertext, a .sha256 sidecar, and manifest.json go to
# /opt/tellomi/backups/daily/ and to r2:tellomi-prod-db-backups/{mysql,etcd,redis,manifests}/.
# Local files older than 90 days are removed. Remote expiry is an R2 lifecycle
# rule; this script does not delete remote objects.
set -euo pipefail

LOG_TAG=tellomi-db-backup
# Public age recipient. The matching identity stays offline. See docs/ops/BACKUP.md.
AGE_RECIPIENT="${TELLOMI_BACKUP_AGE_RECIPIENT:-age1wm306jrc25rl0lavlte3m8ctqn5l5p0lmjnhtgfeeh3nufvljs6q8ypr7t}"
BACKUP_DIR="${TELLOMI_BACKUP_DIR:-/opt/tellomi/backups/daily}"
RETAIN_DAYS="${TELLOMI_BACKUP_RETAIN_DAYS:-90}"
DEPLOY_DIR="${TELLOMI_DEPLOY_DIR:-/opt/tellomi/teamgram-server}"
REDIS_DATA="${TELLOMI_REDIS_DATA:-${DEPLOY_DIR}/data/redis}"
# Bind mount of the etcd container's /etcd-data. The image has etcdctl and
# nothing else, so the snapshot is written there and removed on the host.
ETCD_DATA="${TELLOMI_ETCD_DATA:-${DEPLOY_DIR}/data/etcd}"
ETCD_HOST_SNAP=""
MYSQL_CONTAINER="${TELLOMI_MYSQL_CONTAINER:-mysql}"
ETCD_CONTAINER="${TELLOMI_ETCD_CONTAINER:-etcd}"
REDIS_CONTAINER="${TELLOMI_REDIS_CONTAINER:-redis}"
REMOTE="${TELLOMI_BACKUP_R2_REMOTE:-r2:tellomi-prod-db-backups}"
DRY=0
declare -a MANIFEST_ROWS=()

# shellcheck source=scripts/ops/tellomi-backup-common.sh
source "$(dirname "$(readlink -f "$0")")/tellomi-backup-common.sh"

self_test() {
  local script code mock root old new sentinel out rc
  local src packed listed old_path
  SELF_TMP="$(mktemp -d)"
  script="$(readlink -f "$0")"
  trap 'rm -rf "${SELF_TMP:-}"' EXIT

  if [[ "$AGE_RECIPIENT" != "age1wm306jrc25rl0lavlte3m8ctqn5l5p0lmjnhtgfeeh3nufvljs6q8ypr7t" && -z "${TELLOMI_BACKUP_AGE_RECIPIENT:-}" ]]; then
    backup_die "default age recipient drifted"
  fi
  grep -q 'TELLOMI_BACKUP_RETAIN_DAYS:-90' "$script" || backup_die "local retention default is not 90 days"
  grep -q -- '--s3-no-check-bucket' "$(dirname "$script")/tellomi-backup-common.sh" || backup_die "missing --s3-no-check-bucket"
  grep -q -- '--s3-upload-cutoff' "$(dirname "$script")/tellomi-backup-common.sh" || backup_die "missing --s3-upload-cutoff"

  code="$(
    awk '
      /^self_test\(\) \{/ {skip=1; next}
      skip && /^\}/ {skip=0; next}
      skip {next}
      {print}
    ' "$script"
    cat "$(dirname "$script")/tellomi-backup-common.sh"
  )"
  code="$(printf '%s\n' "$code" | sed -E 's/[[:space:]]+#.*$//; /^[[:space:]]*#/d; /^[[:space:]]*$/d')"
  if printf '%s\n' "$code" | grep -E -n 'FLUSHALL|FLUSHDB|FLUSH |rclone[[:space:]]+delete|rclone[[:space:]]+purge|--min-age' >/dev/null; then
    backup_die "forbidden command in executable lines"
  fi
  if printf '%s\n' "$code" | grep -E -n 'docker exec[^#]*[[:space:]]rm([[:space:]]|$)' >/dev/null; then
    backup_die "docker exec must not run rm; etcd has no rm"
  fi
  if printf '%s\n' "$code" | grep -F -q '/tmp/tellomi-etcd.snapshot'; then
    backup_die "etcd snapshot must not be written to container /tmp"
  fi
  printf '%s\n' "$code" | grep -q 'redis-cli --raw' || backup_die "redis-cli must pass --raw"
  if printf '%s\n' "$code" | grep -E -n 'redis-cli' | grep -E -v 'redis-cli --raw' >/dev/null; then
    backup_die "redis-cli must go through redis_cli"
  fi

  (redis_cli "FLUSH""ALL") >/dev/null 2>&1 && backup_die "FLUSHALL was not refused"
  (redis_cli "FLUSH""DB") >/dev/null 2>&1 && backup_die "FLUSHDB was not refused"
  (redis_cli "CON""FIG") >/dev/null 2>&1 && backup_die "CONFIG was not refused"
  (redis_cli "SA""VE") >/dev/null 2>&1 && backup_die "SAVE was not refused"

  root="${SELF_TMP}/daily"
  mkdir -p "${root}/mysql" "${SELF_TMP}/outside"
  old="${root}/mysql/old.age"
  new="${root}/mysql/new.age"
  sentinel="${SELF_TMP}/outside/sentinel"
  printf 'old\n' >"$old"
  printf 'new\n' >"$new"
  printf 'keep\n' >"$sentinel"
  touch -d '100 days ago' "$old"
  DRY=0 TELLOMI_BACKUP_ALLOW_DIR=1 backup_prune_local "$root" 90
  [[ ! -f "$old" ]] || backup_die "100-day file was kept"
  [[ -f "$new" ]] || backup_die "fresh file was deleted"
  [[ -f "$sentinel" ]] || backup_die "prune escaped the backup dir"
  if (TELLOMI_BACKUP_ALLOW_DIR=1 backup_prune_local /opt/tellomi 90) >/dev/null 2>&1; then
    backup_die "prune accepted a short path"
  fi

  src="${SELF_TMP}/redis-src"
  mkdir -p "${src}/appendonlydir"
  printf 'rdb\n' >"${src}/dump.rdb"
  printf 'aof\n' >"${src}/appendonlydir/appendonly.aof.1.incr.aof"
  printf 'manifest\n' >"${src}/appendonlydir/appendonly.aof.manifest"
  packed="${SELF_TMP}/redis.tar.gz"
  pack_redis_dir "$src" "$packed"
  listed="$(gzip -dc "$packed" | tar -tf - | sort)"
  [[ "$listed" == $'appendonlydir/\nappendonlydir/appendonly.aof.1.incr.aof\nappendonlydir/appendonly.aof.manifest\ndump.rdb' ]] \
    || backup_die "redis archive contents: ${listed}"
  [[ "$(cat "${src}/dump.rdb")" == "rdb" ]] || backup_die "source dump.rdb changed"

  MANIFEST_ROWS=()
  printf 'x\n' >"${SELF_TMP}/obj.age"
  backup_manifest_add mysql teamgram-test.sql.gz.age "$(backup_write_sha256 "${SELF_TMP}/obj.age")" "$(stat -c '%s' "${SELF_TMP}/obj.age")"
  backup_write_manifest "${SELF_TMP}/manifest.json" "2026-11-03T03:15:00Z" "2026-11-03T03:16:00Z" \
    "0123456789abcdef0123456789abcdef01234567" 90 dedicated
  python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); assert d["git_sha"].startswith("0123"); assert d["retain_days"]==90; assert d["files"][0]["sha256"]==open(sys.argv[2]).read().split()[0]; assert d["files"][0]["size"]==2' \
    "${SELF_TMP}/manifest.json" "${SELF_TMP}/obj.age.sha256"

  printf 'RCLONE_CONFIG_R2_TYPE=s3\nRCLONE_CONFIG_R2_ACCESS_KEY_ID=AKIATEST\nRCLONE_CONFIG_R2_SECRET_ACCESS_KEY=SECRETTEST\nRCLONE_CONFIG_R2_ENDPOINT=https://example.r2.cloudflarestorage.com\n' >"${SELF_TMP}/r2.env"
  chmod 600 "${SELF_TMP}/r2.env"
  (
    TELLOMI_BACKUP_R2_ENV="${SELF_TMP}/r2.env"
    backup_load_r2
    [[ "$CRED_SOURCE" == "dedicated" ]]
    [[ "$RCLONE_CONFIG_R2_ACCESS_KEY_ID" == "AKIATEST" ]]
  ) || backup_die "dedicated key was not loaded"

  mock="${SELF_TMP}/bin"
  mkdir -p "$mock"
  cat >"${mock}/docker" <<'EOF'
#!/bin/sh
if [ "$1" = "exec" ] && [ "$3" = "printenv" ]; then
  case "$4" in
    TELLOMI_S3_ACCESS_KEY_ID) printf '%s' "AKIATEST"; exit 0 ;;
    TELLOMI_S3_SECRET_ACCESS_KEY) printf '%s' "SECRETTEST"; exit 0 ;;
    TELLOMI_S3_ENDPOINT) printf '%s' "https://example.r2.cloudflarestorage.com"; exit 0 ;;
  esac
fi
echo "unexpected docker $*" >&2
exit 99
EOF
  chmod 755 "${mock}/docker"
  old_path="$PATH"
  PATH="${mock}:${PATH}"
  out="$(
    {
      TELLOMI_BACKUP_R2_ENV="${SELF_TMP}/missing.env"
      unset TELLOMI_BACKUP_REQUIRE_DEDICATED_KEY
      backup_load_r2
      if [ "$CRED_SOURCE" != "media-fallback" ]; then
        echo "bad source" >&2
        exit 1
      fi
      if [ "$RCLONE_CONFIG_R2_ACCESS_KEY_ID" != "AKIATEST" ]; then
        echo "bad id" >&2
        exit 1
      fi
    } 2>&1
  )" || backup_die "media fallback failed"
  PATH="$old_path"
  printf '%s\n' "$out" | grep -q 'WARNING:' || backup_die "fallback did not warn"
  printf '%s\n' "$out" | grep -q 'SECRETTEST' && backup_die "fallback printed the secret"

  set +e
  out="$(
    {
      TELLOMI_BACKUP_REQUIRE_DEDICATED_KEY=1
      TELLOMI_BACKUP_R2_ENV="${SELF_TMP}/missing.env"
      backup_load_r2
    } 2>&1
  )"
  rc=$?
  set -e
  [[ "$rc" -ne 0 ]] || backup_die "require-dedicated-key did not fail"
  printf '%s\n' "$out" | grep -q 'TELLOMI_BACKUP_REQUIRE_DEDICATED_KEY=1' || backup_die "require-dedicated-key message missing"

  for bin in docker rclone age mysqldump etcdctl redis-cli; do
    cat >"${mock}/${bin}" <<'EOF'
#!/bin/sh
echo "invoked $0 $*" >&2
exit 99
EOF
    chmod 755 "${mock}/${bin}"
  done
  mkdir -p "${root}/mysql" "${root}/etcd" "${root}/redis" "${root}/manifests"
  printf 'stale\n' >"${root}/mysql/stale.age"
  touch -d '100 days ago' "${root}/mysql/stale.age"
  PATH="${mock}:${old_path}"
  out="$(
    {
      env TELLOMI_BACKUP_SKIP_TOOLS=1 \
        TELLOMI_BACKUP_ALLOW_DIR=1 \
        TELLOMI_BACKUP_DIR="$root" \
        TELLOMI_BACKUP_R2_ENV="${SELF_TMP}/r2.env" \
        TELLOMI_DEPLOY_DIR="${SELF_TMP}/no-git" \
        bash "$script" --dry-run
    } 2>&1
  )" || backup_die "dry-run failed"
  PATH="$old_path"
  printf '%s\n' "$out" | grep -q -- '--s3-no-check-bucket' || backup_die "dry-run omitted --s3-no-check-bucket"
  printf '%s\n' "$out" | grep -q -- '--s3-upload-cutoff 5G' || backup_die "dry-run omitted upload cutoff"
  printf '%s\n' "$out" | grep -q -- '--s3-use-presigned-request' || backup_die "dry-run omitted presigned upload"
  printf '%s\n' "$out" | grep -q -- '--s3-no-head' || backup_die "dry-run omitted --s3-no-head"
  printf '%s\n' "$out" | grep -q 'BGSAVE' || backup_die "dry-run omitted BGSAVE"
  printf '%s\n' "$out" | grep -q 'etcdctl snapshot save' || backup_die "dry-run omitted etcd snapshot"
  printf '%s\n' "$out" | grep -q 'mysqldump' || backup_die "dry-run omitted mysqldump"
  printf '%s\n' "$out" | grep -q 'would delete' || backup_die "dry-run omitted local retention"
  printf '%s\n' "$out" | grep -E -q 'FLUSH|rclone delete' && backup_die "dry-run showed a forbidden command"
  [[ -f "${root}/mysql/stale.age" ]] || backup_die "dry-run deleted a file"
  printf '%s\n' "$out" | grep -q 'SECRETTEST' && backup_die "dry-run printed the secret"
  printf '%s\n' "$out" | grep -q 'invoked' && backup_die "dry-run executed docker, rclone, or age"

  echo "tellomi-db-backup: self-test ok"
}

SELFTEST=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run) DRY=1 ;;
    --self-test) SELFTEST=1 ;;
    -h|--help)
      echo "usage: $0 [--dry-run|--self-test]" >&2
      exit 0
      ;;
    *)
      echo "usage: $0 [--dry-run|--self-test]" >&2
      exit 2
      ;;
  esac
  shift
done

require_tools() {
  local t
  for t in docker gzip age rclone sha256sum tar find flock stat; do
    command -v "$t" >/dev/null 2>&1 || backup_die "missing tool: ${t}"
  done
}

redis_cli() {
  local cmd="$1" out rc
  case "$cmd" in
    BGSAVE|LASTSAVE) ;;
    *) backup_die "refusing redis command ${cmd}" ;;
  esac
  set +e
  out="$(docker exec "$REDIS_CONTAINER" redis-cli --raw "$cmd" 2>&1)"
  rc=$?
  set -e
  out="${out//$'\r'/}"
  if [[ "$cmd" == "BGSAVE" && "$rc" -ne 0 && "$out" == *"already in progress"* ]]; then
    backup_log "BGSAVE already in progress"
    return 0
  fi
  if [[ "$rc" -ne 0 ]]; then
    backup_die "redis command ${cmd} failed (${rc})"
  fi
  printf '%s' "$out"
}

# pack_redis_dir tars a read-only copy of dump.rdb and appendonlydir.
pack_redis_dir() {
  local src="$1" dest="$2" copy rc
  [[ -f "${src}/dump.rdb" ]] || backup_die "dump.rdb missing under ${src}"
  [[ -d "${src}/appendonlydir" ]] || backup_die "appendonlydir missing under ${src}"
  copy="$(mktemp -d)"
  cp -a -- "${src}/dump.rdb" "${copy}/dump.rdb"
  cp -a -- "${src}/appendonlydir" "${copy}/appendonlydir"
  set +e
  tar -C "$copy" -cf - dump.rdb appendonlydir | gzip -c >"$dest"
  rc=$?
  set -e
  rm -rf "$copy"
  [[ "$rc" -eq 0 ]] || backup_die "redis tar failed"
}

encrypt_file() {
  local plain="$1" dest="$2"
  gzip -dc "$plain" >/dev/null
  age -r "$AGE_RECIPIENT" -o "$dest" <"$plain"
  chmod 600 "$dest"
}

finish_object() {
  local prefix="$1" path="$2" name sum size
  name="$(basename "$path")"
  sum="$(backup_write_sha256 "$path")"
  size="$(stat -c '%s' "$path")"
  chmod 600 "$path"
  backup_manifest_add "$prefix" "$name" "$sum" "$size"
  if [[ "$DRY" -eq 1 ]]; then
    backup_log "dry-run: rclone copyto ${path} ${REMOTE}/${prefix}/${name} ${RCLONE_S3_FLAGS[*]}"
    backup_log "dry-run: rclone copyto ${path}.sha256 ${REMOTE}/${prefix}/${name}.sha256 ${RCLONE_S3_FLAGS[*]}"
    return 0
  fi
  backup_rclone copyto "$path" "${REMOTE}/${prefix}/${name}"
  backup_rclone copyto "${path}.sha256" "${REMOTE}/${prefix}/${name}.sha256"
  backup_log "uploaded ${prefix}/${name} (${size} bytes)"
}

# mysql:8.0 has /bin/sh and mysqldump. redis:7-alpine has redis-cli.
# quay.io/coreos/etcd has etcd and etcdctl only: no sh, no rm, no tar.
backup_mysql() {
  local dest="$1" plain="${WORK}/mysql.sql.gz"
  docker exec "$MYSQL_CONTAINER" sh -c 'exec mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" --single-transaction --quick --routines --events --triggers --no-tablespaces --set-gtid-purged=OFF --databases teamgram' \
    | gzip -c >"$plain"
  encrypt_file "$plain" "$dest"
  rm -f "$plain"
}

backup_etcd() {
  local dest="$1" plain="${WORK}/etcd.db.gz"
  [[ -d "$ETCD_DATA" ]] || backup_die "etcd data dir ${ETCD_DATA} is not a directory (set TELLOMI_ETCD_DATA to the host path of /etcd-data)"
  ETCD_HOST_SNAP="${ETCD_DATA}/tellomi-etcd.snapshot"
  [[ "$(basename "$ETCD_HOST_SNAP")" == "tellomi-etcd.snapshot" ]] || backup_die "refusing to use ${ETCD_HOST_SNAP}"
  rm -f -- "$ETCD_HOST_SNAP"
  docker exec "$ETCD_CONTAINER" etcdctl --endpoints=http://127.0.0.1:2379 snapshot save /etcd-data/tellomi-etcd.snapshot
  if [[ ! -s "$ETCD_HOST_SNAP" ]]; then
    backup_die "etcd snapshot was not on the host at ${ETCD_HOST_SNAP}. TELLOMI_ETCD_DATA must be the bind mount of /etcd-data; remove /etcd-data/tellomi-etcd.snapshot from that mount if etcdctl wrote it elsewhere."
  fi
  gzip -c "$ETCD_HOST_SNAP" >"$plain"
  rm -f -- "$ETCD_HOST_SNAP"
  ETCD_HOST_SNAP=""
  encrypt_file "$plain" "$dest"
  rm -f "$plain"
}

backup_redis() {
  local dest="$1" plain="${WORK}/redis.tar.gz" before now deadline
  before="$(redis_cli LASTSAVE)"
  [[ "$before" =~ ^[0-9]+$ ]] || backup_die "LASTSAVE was not a timestamp"
  redis_cli BGSAVE >/dev/null
  deadline=$((SECONDS + ${TELLOMI_BGSAVE_TIMEOUT:-180}))
  while true; do
    now="$(redis_cli LASTSAVE)"
    if [[ "$now" =~ ^[0-9]+$ && "$now" != "$before" ]]; then
      break
    fi
    if (( SECONDS >= deadline )); then
      backup_die "BGSAVE did not advance LASTSAVE within ${TELLOMI_BGSAVE_TIMEOUT:-180}s"
    fi
    sleep 1
  done
  pack_redis_dir "$REDIS_DATA" "$plain"
  encrypt_file "$plain" "$dest"
  rm -f "$plain"
}

lock_path() {
  if [[ -w /run ]]; then
    printf '/run/tellomi-db-backup.lock'
    return 0
  fi
  printf '%s/tellomi-db-backup.lock' "${TMPDIR:-/tmp}"
}

cleanup_work() {
  [[ -n "${WORK:-}" && -d "$WORK" ]] && rm -rf "$WORK"
  # Drop a snapshot left in the etcd data dir if gzip or age fails mid-run.
  if [[ -n "${ETCD_HOST_SNAP:-}" && "$(basename "$ETCD_HOST_SNAP")" == "tellomi-etcd.snapshot" ]]; then
    rm -f -- "$ETCD_HOST_SNAP"
  fi
}

main() {
  local stamp started finished sha manifest lock
  [[ "$AGE_RECIPIENT" =~ ^age1[a-z0-9]{20,}$ ]] || backup_die "age recipient is not an age1 public key"
  [[ "$RETAIN_DAYS" =~ ^[0-9]+$ ]] || backup_die "TELLOMI_BACKUP_RETAIN_DAYS must be an integer"
  if ! backup_dir_allowed "$BACKUP_DIR"; then
    backup_die "TELLOMI_BACKUP_DIR must be /opt/tellomi/backups or a directory under it"
  fi
  if [[ "${TELLOMI_BACKUP_SKIP_TOOLS:-}" != "1" ]]; then
    require_tools
  fi
  backup_set_rclone_flags
  umask 077
  mkdir -p "${BACKUP_DIR}/mysql" "${BACKUP_DIR}/etcd" "${BACKUP_DIR}/redis" "${BACKUP_DIR}/manifests"
  chmod 700 "$BACKUP_DIR" "${BACKUP_DIR}/mysql" "${BACKUP_DIR}/etcd" "${BACKUP_DIR}/redis" "${BACKUP_DIR}/manifests" || true
  lock="$(lock_path)"
  exec 9>"$lock"
  flock -w 5 9 || backup_die "another backup holds ${lock}"
  backup_load_r2
  stamp="$(date -u +%Y%m%dT%H%M%SZ)"
  started="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  sha="$(TELLOMI_DEPLOY_DIR="$DEPLOY_DIR" backup_git_sha)"
  backup_log "start stamp=${stamp} git=${sha} dry_run=${DRY} retain_days=${RETAIN_DAYS} credentials=${CRED_SOURCE}"
  if [[ "$DRY" -eq 1 ]]; then
    backup_log "dry-run: docker exec ${MYSQL_CONTAINER} mysqldump --single-transaction --databases teamgram | gzip | age -r ${AGE_RECIPIENT}"
    backup_log "dry-run: docker exec ${ETCD_CONTAINER} etcdctl snapshot save /etcd-data/tellomi-etcd.snapshot; delete ${ETCD_DATA}/tellomi-etcd.snapshot on the host"
    backup_log "dry-run: BGSAVE; wait for LASTSAVE; read-only tar of ${REDIS_DATA}/dump.rdb and appendonlydir | gzip | age"
    backup_log "dry-run: rclone copyto ${REMOTE}/mysql/ ${REMOTE}/etcd/ ${REMOTE}/redis/ ${REMOTE}/manifests/ ${RCLONE_S3_FLAGS[*]}"
    backup_prune_local "$BACKUP_DIR" "$RETAIN_DAYS"
    backup_log "done dry-run stamp=${stamp}"
    return 0
  fi
  WORK="$(mktemp -d "${TMPDIR:-/tmp}/tellomi-db-backup.XXXXXX")"
  chmod 700 "$WORK"
  trap cleanup_work EXIT

  backup_mysql "${BACKUP_DIR}/mysql/teamgram-${stamp}.sql.gz.age"
  finish_object mysql "${BACKUP_DIR}/mysql/teamgram-${stamp}.sql.gz.age"

  backup_etcd "${BACKUP_DIR}/etcd/etcd-${stamp}.db.gz.age"
  finish_object etcd "${BACKUP_DIR}/etcd/etcd-${stamp}.db.gz.age"

  backup_redis "${BACKUP_DIR}/redis/redis-${stamp}.tar.gz.age"
  finish_object redis "${BACKUP_DIR}/redis/redis-${stamp}.tar.gz.age"

  finished="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  manifest="${BACKUP_DIR}/manifests/${stamp}.json"
  backup_write_manifest "$manifest" "$started" "$finished" "$sha" "$RETAIN_DAYS" "$CRED_SOURCE"
  if [[ "$DRY" -eq 1 ]]; then
    backup_log "dry-run: rclone copyto ${manifest} ${REMOTE}/manifests/${stamp}.json ${RCLONE_S3_FLAGS[*]}"
  else
    backup_rclone copyto "$manifest" "${REMOTE}/manifests/${stamp}.json"
    backup_log "uploaded manifests/${stamp}.json"
  fi
  backup_prune_local "$BACKUP_DIR" "$RETAIN_DAYS"
  backup_log "done stamp=${stamp} credentials=${CRED_SOURCE}"
}

if [[ "$SELFTEST" -eq 1 ]]; then
  self_test
  exit 0
fi
main
