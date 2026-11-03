#!/usr/bin/env bash
# Monthly restore drill on a fresh Docker host (D-34, D-53, M7).
# Never run this on Chengdu. See docs/ops/RESTORE_DRILL.md.
#
#   TELLOMI_IMAGE_TAG=<sha7> TELLOMI_DRILL_CONFIRM=restore-drill \
#     TELLOMI_BACKUP_AGE_IDENTITY=/path/to/identity \
#     TELLOMI_BACKUP_R2_ENV=/path/to/readonly.env \
#     bash scripts/ops/tellomi-restore-drill.sh
#
#   bash scripts/ops/tellomi-restore-drill.sh --dry-run
#   bash scripts/ops/tellomi-restore-drill.sh --self-test
set -euo pipefail

LOG_TAG=tellomi-restore-drill
PROJECT=tellomi-drill
ETCD_IMAGE="${TELLOMI_DRILL_ETCD_IMAGE:-quay.io/coreos/etcd:v3.5.11}"
ROOT="${TELLOMI_DEPLOY_DIR:-$(pwd)}"
RECORD="${TELLOMI_DRILL_RECORD:-${ROOT}/drill-record.md}"
DRY=0
SELFTEST=0
STARTED_EPOCH=0

# shellcheck source=scripts/ops/tellomi-backup-common.sh
source "$(dirname "$(readlink -f "$0")")/tellomi-backup-common.sh"

step_die() {
  echo "ERROR: $*" >&2
  return 1
}

production_guard() {
  if [[ "$PROJECT" == "tellomi" ]]; then
    backup_die "refusing compose project tellomi (production)"
  fi
  if ! command -v docker >/dev/null 2>&1; then
    if [[ "${TELLOMI_DRILL_SKIP_DOCKER:-}" == "1" ]]; then
      return 0
    fi
    backup_die "docker is required"
  fi
  if docker ps -a --format '{{.Names}}' | grep -qx 'tellomi-teamgram-1'; then
    backup_die "refusing: tellomi-teamgram-1 exists; this looks like production"
  fi
  local c proj
  for c in mysql etcd redis kafka; do
    if docker inspect "$c" >/dev/null 2>&1; then
      proj="$(docker inspect -f '{{index .Config.Labels "com.docker.compose.project"}}' "$c")"
      if [[ "$proj" != "$PROJECT" ]]; then
        backup_die "refusing: container ${c} belongs to project ${proj:-unknown}"
      fi
    fi
  done
}

compose() {
  docker compose -p "$PROJECT" -f "${ROOT}/docker-compose-env.yaml" -f "${ROOT}/docker-compose.yaml" "$@"
}

record_open() {
  local when
  when="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  cat >"$RECORD" <<EOF
# Restore drill record

| Field | Value |
|---|---|
| Date (UTC) | ${when} |
| Operator | ${TELLOMI_DRILL_OPERATOR:-} |
| Drill host | $(hostname) |
| Image tag (7 hex) | ${TELLOMI_IMAGE_TAG:-} |
| Backup stamp | |
| Git SHA in the manifest | |
| Started (UTC) | ${when} |
| Finished (UTC) | |
| Total seconds | |
| Within 2 hours | |

| Step | Start (UTC) | End (UTC) | Seconds | Result | Notes |
|---|---|---|---|---|---|
EOF
}

record_step() {
  printf '| %s | %s | %s | %s | %s | %s |\n' "$1" "$2" "$3" "$4" "$5" "$6" >>"$RECORD"
}

run_step() {
  local name="$1" note="$2"
  shift 2
  local start_e start_h end_e end_h rc
  start_e="$(date +%s)"
  start_h="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  set +e
  if [[ "$DRY" -eq 1 ]]; then
    backup_log "dry-run: ${name}: ${note}"
    rc=0
  else
    "$@"
    rc=$?
  fi
  set -e
  end_e="$(date +%s)"
  end_h="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  if [[ "$rc" -eq 0 ]]; then
    record_step "$name" "$start_h" "$end_h" "$((end_e - start_e))" pass "$note"
  else
    record_step "$name" "$start_h" "$end_h" "$((end_e - start_e))" fail "$note"
    backup_die "step ${name} failed"
  fi
}

finish_record() {
  local end total within
  end="$(date +%s)"
  total="$((end - STARTED_EPOCH))"
  if (( total <= 7200 )); then
    within=yes
  else
    within=no
  fi
  {
    echo
    echo "Finished (UTC): $(date -u +%Y-%m-%dT%H:%M:%SZ)"
    echo "Total seconds: ${total}"
    echo "Within 2 hours: ${within}"
  } >>"$RECORD"
  backup_log "drill record ${RECORD} total=${total}s within_2h=${within}"
  if [[ "$within" != "yes" && "$DRY" -eq 0 ]]; then
    backup_die "drill exceeded 2 hours (${total}s)"
  fi
}

step_tools() {
  local t
  for t in age rclone docker python3 go gzip tar git; do
    command -v "$t" >/dev/null 2>&1 || step_die "missing tool: ${t}"
  done
}

step_pull() {
  cd "$ROOT"
  git fetch -q origin master
  git checkout -q --detach "$TELLOMI_IMAGE_TAG"
  export TELLOMI_IMAGE_TAG
  compose pull teamgram asr
}

newest_object() {
  local prefix="$1" listing
  listing="${WORK}/${prefix}.json"
  backup_rclone lsjson "${TELLOMI_BACKUP_R2_REMOTE:-r2:tellomi-prod-db-backups}/${prefix}/" >"$listing"
  python3 - "$listing" <<'PY'
import json, sys
items = json.load(open(sys.argv[1]))
best = None
for it in items:
    if it.get("IsDir"):
        continue
    name = it.get("Name") or ""
    if name.endswith(".age") and not name.endswith(".sha256"):
        mod = it.get("ModTime") or ""
        if best is None or mod > best[0]:
            best = (mod, name)
if best is None:
    sys.exit(1)
print(best[1])
PY
}

step_fetch() {
  local prefix name
  export TELLOMI_BACKUP_REQUIRE_DEDICATED_KEY=1
  backup_load_r2
  mkdir -p "${WORK}/in"
  for prefix in mysql etcd redis; do
    name="$(newest_object "$prefix")"
    backup_rclone copyto "${TELLOMI_BACKUP_R2_REMOTE:-r2:tellomi-prod-db-backups}/${prefix}/${name}" "${WORK}/in/${name}"
    backup_rclone copyto "${TELLOMI_BACKUP_R2_REMOTE:-r2:tellomi-prod-db-backups}/${prefix}/${name}.sha256" "${WORK}/in/${name}.sha256"
    (cd "${WORK}/in" && sha256sum -c "${name}.sha256") || step_die "sha256 mismatch for ${name}"
  done
}

step_decrypt() {
  local agefile plain
  mkdir -p "${WORK}/plain"
  shopt -s nullglob
  for agefile in "${WORK}/in/"*.age; do
    plain="${WORK}/plain/$(basename "$agefile" .age)"
    age -d -i "$TELLOMI_BACKUP_AGE_IDENTITY" -o "$plain" "$agefile" || step_die "age decrypt failed"
  done
  shopt -u nullglob
}

step_mysql() {
  local dump
  dump="$(echo "${WORK}/plain/"teamgram-*.sql.gz)"
  [[ -f "$dump" ]] || step_die "mysql dump missing"
  compose up -d mysql
  local i
  for ((i = 0; i < 60; i++)); do
    # The password is the container's MYSQL_ROOT_PASSWORD, not a host argument.
    if docker exec mysql sh -c 'mysqladmin --silent ping -uroot -p"$MYSQL_ROOT_PASSWORD"' >/dev/null 2>&1; then
      break
    fi
    sleep 2
  done
  docker exec mysql sh -c 'mysqladmin --silent ping -uroot -p"$MYSQL_ROOT_PASSWORD"' || step_die "mysql did not become ready"
  gzip -dc "$dump" | docker exec -i mysql sh -c 'exec mysql -uroot -p"$MYSQL_ROOT_PASSWORD"' || step_die "mysql import failed"
}

step_redis() {
  local archive n
  archive="$(echo "${WORK}/plain/"redis-*.tar.gz)"
  [[ -f "$archive" ]] || step_die "redis archive missing"
  mkdir -p "${ROOT}/data/redis"
  rm -rf "${ROOT}/data/redis/dump.rdb" "${ROOT}/data/redis/appendonlydir"
  tar -xzf "$archive" -C "${ROOT}/data/redis"
  [[ -f "${ROOT}/data/redis/dump.rdb" ]] || step_die "dump.rdb missing after extract"
  [[ -d "${ROOT}/data/redis/appendonlydir" ]] || step_die "appendonlydir missing after extract"
  compose up -d redis
  local i
  for ((i = 0; i < 30; i++)); do
    if docker exec redis redis-cli ping | grep -q PONG; then
      break
    fi
    sleep 1
  done
  n="$(docker exec redis redis-cli --scan --pattern 'pts_updates_ngen_*' | wc -l)"
  backup_log "pts counters ${n}"
  [[ "$n" -gt 0 ]] || step_die "no pts_updates_ngen_ keys after restore"
  n="$(docker exec redis redis-cli --scan --pattern 'message_box_ngen_*' | wc -l)"
  backup_log "message box counters ${n}"
  [[ "$n" -gt 0 ]] || step_die "no message_box_ngen_ keys after restore"
}

step_etcd() {
  local snap
  snap="$(echo "${WORK}/plain/"etcd-*.db.gz)"
  [[ -f "$snap" ]] || step_die "etcd snapshot missing"
  mkdir -p "${ROOT}/data"
  gzip -dc "$snap" >"${ROOT}/data/etcd.snapshot"
  rm -rf "${ROOT}/data/etcd-restore" "${ROOT}/data/etcd"
  docker run --rm -v "${ROOT}/data:/work" --entrypoint etcdctl "$ETCD_IMAGE" \
    snapshot restore /work/etcd.snapshot \
    --name node1 \
    --initial-cluster node1=http://etcd:2380 \
    --initial-advertise-peer-urls http://etcd:2380 \
    --data-dir /work/etcd-restore || step_die "etcd snapshot restore failed"
  mv "${ROOT}/data/etcd-restore" "${ROOT}/data/etcd"
  rm -f "${ROOT}/data/etcd.snapshot"
  compose up -d etcd
}

step_up() {
  compose up -d
}

e2e_test() {
  local run="$1"
  (cd "${ROOT}/e2e/client" && TELLOMI_E2E=1 TELLOMI_E2E_ADDR="${TELLOMI_E2E_ADDR:-127.0.0.1:10443}" \
    go test -count=1 -timeout 20m -run "$run" .)
}

step_handshake() { e2e_test 'TestInitConnectionInvokeWithLayer$'; }
step_login() { e2e_test 'TestHelpGetConfig$'; }
step_message() { e2e_test 'TestPrivateChatReadOutbox$'; }

drill() {
  local sha secrets identity
  sha="${TELLOMI_IMAGE_TAG:-}"
  [[ "$sha" =~ ^[0-9a-f]{7}$ ]] || backup_die "set TELLOMI_IMAGE_TAG to the 7-character image sha"
  production_guard
  if [[ "$DRY" -eq 0 && "${TELLOMI_DRILL_CONFIRM:-}" != "restore-drill" ]]; then
    backup_die "set TELLOMI_DRILL_CONFIRM=restore-drill on a fresh host"
  fi
  secrets="${TELLOMI_BACKUP_R2_ENV:-/opt/tellomi/secrets/db-backup-r2.env}"
  identity="${TELLOMI_BACKUP_AGE_IDENTITY:-}"
  if [[ "$DRY" -eq 0 ]]; then
    [[ -n "$identity" && -f "$identity" ]] || backup_die "set TELLOMI_BACKUP_AGE_IDENTITY to the offline age identity"
    [[ -f "$secrets" ]] || backup_die "read-only R2 env file ${secrets} is missing"
  fi
  umask 077
  WORK="$(mktemp -d "${TMPDIR:-/tmp}/tellomi-restore-drill.XXXXXX")"
  chmod 700 "$WORK"
  trap 'rm -rf "${WORK:-}"' EXIT
  STARTED_EPOCH="$(date +%s)"
  record_open
  run_step install-tools "age rclone docker python3 go" step_tools
  run_step pull-images "checkout ${sha} and pull teamgram asr" step_pull
  run_step fetch-backups "read-only token, newest mysql etcd redis" step_fetch
  run_step decrypt "age -d with the offline identity" step_decrypt
  run_step restore-mysql "import teamgram dump" step_mysql
  run_step restore-redis "extract dump.rdb and appendonlydir, scan counters, no FLUSH" step_redis
  run_step restore-etcd "etcdctl snapshot restore, then start etcd" step_etcd
  run_step compose-up "project ${PROJECT}" step_up
  run_step handshake "TestInitConnectionInvokeWithLayer" step_handshake
  run_step login "TestHelpGetConfig (auth.signIn)" step_login
  run_step message "TestPrivateChatReadOutbox" step_message
  finish_record
}

self_test() {
  local script code out
  SELF_TMP="$(mktemp -d)"
  script="$(readlink -f "$0")"
  trap 'rm -rf "${SELF_TMP:-}"' EXIT
  code="$(
    awk '
      /^self_test\(\) \{/ {skip=1; next}
      skip && /^\}/ {skip=0; next}
      skip {next}
      {print}
    ' "$script" | sed -E 's/[[:space:]]+#.*$//; /^[[:space:]]*#/d; /^[[:space:]]*$/d'
  )"
  if printf '%s\n' "$code" | grep -E -n 'FLUSHALL|FLUSHDB|FLUSH ' >/dev/null; then
    backup_die "drill must not flush redis"
  fi
  printf '%s\n' "$code" | grep -q 'redis-cli --scan' || backup_die "redis check must be a scan"
  if (PROJECT=tellomi production_guard); then
    backup_die "project tellomi was allowed"
  fi

  mkdir -p "${SELF_TMP}/bin"
  cat >"${SELF_TMP}/bin/docker" <<'EOF'
#!/bin/sh
if [ "$1" = "ps" ]; then
  echo tellomi-teamgram-1
  exit 0
fi
exit 0
EOF
  chmod 755 "${SELF_TMP}/bin/docker"
  if (PATH="${SELF_TMP}/bin:${PATH}" production_guard); then
    backup_die "production container was allowed"
  fi

  cat >"${SELF_TMP}/bin/docker" <<'EOF'
#!/bin/sh
if [ "$1" = "ps" ]; then
  exit 0
fi
if [ "$1" = "inspect" ]; then
  exit 1
fi
echo "unexpected docker $*" >&2
exit 99
EOF
  cat >"${SELF_TMP}/bin/age" <<'EOF'
#!/bin/sh
echo "age invoked" >&2
exit 99
EOF
  cat >"${SELF_TMP}/bin/rclone" <<'EOF'
#!/bin/sh
echo "rclone invoked" >&2
exit 99
EOF
  chmod 755 "${SELF_TMP}/bin/docker" "${SELF_TMP}/bin/age" "${SELF_TMP}/bin/rclone"
  out="$(
    {
      PATH="${SELF_TMP}/bin:${PATH}" \
        TELLOMI_IMAGE_TAG=abcdef1 \
        TELLOMI_DRILL_SKIP_DOCKER=0 \
        TELLOMI_DRILL_RECORD="${SELF_TMP}/record.md" \
        TELLOMI_DEPLOY_DIR="$SELF_TMP" \
        bash "$script" --dry-run
    } 2>&1
  )" || backup_die "dry-run failed"
  printf '%s\n' "$out" | grep -q 'pull-images' || backup_die "dry-run omitted pull-images"
  printf '%s\n' "$out" | grep -q 'TestInitConnectionInvokeWithLayer' || backup_die "dry-run omitted handshake"
  printf '%s\n' "$out" | grep -q 'TestHelpGetConfig' || backup_die "dry-run omitted login"
  printf '%s\n' "$out" | grep -q 'TestPrivateChatReadOutbox' || backup_die "dry-run omitted message"
  printf '%s\n' "$out" | grep -q 'no FLUSH' || backup_die "dry-run omitted the redis warning"
  printf '%s\n' "$out" | grep -E -q 'age invoked|rclone invoked|unexpected docker' && backup_die "dry-run executed a tool"
  grep -q 'Within 2 hours: yes' "${SELF_TMP}/record.md" || backup_die "record missing the 2 hour line"
  grep -q '| handshake |' "${SELF_TMP}/record.md" || backup_die "record missing handshake"

  if (PATH="${SELF_TMP}/bin:${PATH}" TELLOMI_IMAGE_TAG=abcdef1 TELLOMI_DRILL_RECORD="${SELF_TMP}/nope.md" TELLOMI_DEPLOY_DIR="$SELF_TMP" bash "$script"); then
    backup_die "real run without confirm was allowed"
  fi
  echo "tellomi-restore-drill: self-test ok"
}

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

if [[ "$SELFTEST" -eq 1 ]]; then
  self_test
  exit 0
fi
drill
