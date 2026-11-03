#!/usr/bin/env bash
# Confirm last night's backup landed (D-34, D-53, M7). No decryption.
#
#   tellomi-backup-verify.sh
#   tellomi-backup-verify.sh --self-test
#
# Cron: scripts/ops/tellomi-backup-verify.cron at 04:00.
# Lists the newest .age object under mysql/, etcd/, and redis/, checks it is
# under TELLOMI_BACKUP_MAX_AGE_HOURS (default 26), the sha256 sidecar matches,
# the age header is well formed, and the size is inside the configured bounds.
# Logs to syslog with tag tellomi-backup-verify.
set -euo pipefail

LOG_TAG=tellomi-backup-verify
REMOTE="${TELLOMI_BACKUP_R2_REMOTE:-r2:tellomi-prod-db-backups}"
MAX_AGE_HOURS="${TELLOMI_BACKUP_MAX_AGE_HOURS:-26}"
MIN_BYTES="${TELLOMI_BACKUP_MIN_BYTES:-1024}"
MAX_BYTES="${TELLOMI_BACKUP_MAX_BYTES:-8589934592}"

# shellcheck source=scripts/ops/tellomi-backup-common.sh
source "$(dirname "$(readlink -f "$0")")/tellomi-backup-common.sh"

self_test() {
  local script code sum name size mod now_iso old_iso
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
  if printf '%s\n' "$code" | grep -E -n 'age[[:space:]]+(-d|--decrypt|-i)|--decrypt' >/dev/null; then
    backup_die "verifier must not decrypt"
  fi
  grep -q 'tellomi-backup-verify' "$script" || backup_die "syslog tag missing"

  # Real age files end the header with "--- <base64 MAC>", not a bare "---".
  printf '%s\n' \
    'age-encryption.org/v1' \
    '-> X25519 AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA' \
    'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA' \
    '--- mac' \
    'ciphertext-not-decrypted' >"${SELF_TMP}/good.age"
  backup_age_header_ok "${SELF_TMP}/good.age" || backup_die "valid age header rejected"
  printf '%s\n' \
    'age-encryption.org/v1' \
    '-> X25519 AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA' \
    '---mac' >"${SELF_TMP}/nospace.age"
  if backup_age_header_ok "${SELF_TMP}/nospace.age"; then
    backup_die "age mac line without a space was accepted"
  fi
  if command -v age >/dev/null 2>&1; then
    printf 'header-check\n' | age -r age1wm306jrc25rl0lavlte3m8ctqn5l5p0lmjnhtgfeeh3nufvljs6q8ypr7t -o "${SELF_TMP}/real.age"
    backup_age_header_ok "${SELF_TMP}/real.age" || backup_die "real age file was rejected"
  fi
  printf 'not-age\n' >"${SELF_TMP}/bad.age"
  if backup_age_header_ok "${SELF_TMP}/bad.age"; then
    backup_die "invalid age header accepted"
  fi
  printf '%s\n' 'age-encryption.org/v1' 'no recipient' >"${SELF_TMP}/short.age"
  if backup_age_header_ok "${SELF_TMP}/short.age"; then
    backup_die "truncated age header accepted"
  fi

  name="teamgram-test.sql.gz.age"
  cp "${SELF_TMP}/good.age" "${SELF_TMP}/${name}"
  sum="$(backup_write_sha256 "${SELF_TMP}/${name}")"
  size="$(stat -c '%s' "${SELF_TMP}/${name}")"
  MIN_BYTES=1
  now_iso="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  old_iso="$(date -u -d '48 hours ago' +%Y-%m-%dT%H:%M:%SZ)"

  export MOCK_DIR="$SELF_TMP"
  export MOCK_LSJSON="${SELF_TMP}/ls.json"
  write_lsjson "$name" "$size" "$now_iso" >"$MOCK_LSJSON"
  mock_rclone "${SELF_TMP}/bin"
  PATH="${SELF_TMP}/bin:${PATH}"
  TELLOMI_BACKUP_MIN_BYTES=100
  TELLOMI_BACKUP_R2_ENV="${SELF_TMP}/r2.env"
  write_r2_env "${SELF_TMP}/r2.env"
  check_prefix mysql || backup_die "fresh object should pass"
  backup_log "self-test fresh object passed"

  write_lsjson "$name" "$size" "$old_iso" >"${SELF_TMP}/ls.json"
  if check_prefix mysql; then
    backup_die "48h object should fail"
  fi

  write_lsjson "$name" "$size" "$now_iso" >"${SELF_TMP}/ls.json"
  printf '0000000000000000000000000000000000000000000000000000000000000000  %s\n' "$name" >"${SELF_TMP}/${name}.sha256"
  if check_prefix mysql; then
    backup_die "sha256 mismatch should fail"
  fi
  printf '%s  %s\n' "$sum" "$name" >"${SELF_TMP}/${name}.sha256"

  write_lsjson "$name" 0 "$now_iso" >"${SELF_TMP}/ls.json"
  if check_prefix mysql; then
    backup_die "zero size should fail"
  fi

  write_lsjson "$name" 99999999999 "$now_iso" >"${SELF_TMP}/ls.json"
  if check_prefix mysql; then
    backup_die "huge size should fail"
  fi

  cp "${SELF_TMP}/bad.age" "${SELF_TMP}/${name}"
  sum="$(backup_write_sha256 "${SELF_TMP}/${name}")"
  size="$(stat -c '%s' "${SELF_TMP}/${name}")"
  write_lsjson "$name" "$size" "$now_iso" >"${SELF_TMP}/ls.json"
  if check_prefix mysql; then
    backup_die "bad age header should fail"
  fi

  echo "tellomi-backup-verify: self-test ok"
}

write_r2_env() {
  local path="$1"
  printf '%s\n' \
    'RCLONE_CONFIG_R2_TYPE=s3' \
    'RCLONE_CONFIG_R2_ACCESS_KEY_ID=AKIATEST' \
    'RCLONE_CONFIG_R2_SECRET_ACCESS_KEY=SECRETTEST' \
    'RCLONE_CONFIG_R2_ENDPOINT=https://example.r2.cloudflarestorage.com' >"$path"
  chmod 600 "$path"
}

write_lsjson() {
  local name="$1" size="$2" mod="$3"
  printf '%s\n' "[{\"Name\":\"${name}\",\"Path\":\"${name}\",\"Size\":${size},\"ModTime\":\"${mod}\",\"IsDir\":false},{\"Name\":\"${name}.sha256\",\"Path\":\"${name}.sha256\",\"Size\":80,\"ModTime\":\"${mod}\",\"IsDir\":false}]"
}

mock_rclone() {
  local bin="$1"
  mkdir -p "$bin"
  cat >"${bin}/rclone" <<'EOF'
#!/bin/sh
set -eu
if [ "$1" = "lsjson" ]; then
  cat "$MOCK_LSJSON"
  exit 0
fi
if [ "$1" = "copyto" ]; then
  base=$(basename "$2")
  cp "$MOCK_DIR/$base" "$3"
  exit 0
fi
echo "unexpected rclone $*" >&2
exit 99
EOF
  chmod 755 "${bin}/rclone"
}

if [[ "${1:-}" == "--self-test" ]]; then
  SELFTEST=1
else
  SELFTEST=0
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --self-test) SELFTEST=1 ;;
      -h|--help)
        echo "usage: $0 [--self-test]" >&2
        exit 0
        ;;
      *)
        echo "usage: $0 [--self-test]" >&2
        exit 2
        ;;
    esac
    shift
  done
fi

require_tools() {
  local t
  for t in rclone sha256sum python3 stat flock; do
    command -v "$t" >/dev/null 2>&1 || backup_die "missing tool: ${t}"
  done
}

# newest_age reads an lsjson file ($1) and prints name, size, modtime.
newest_age() {
  python3 - "$1" "$MAX_AGE_HOURS" "$MIN_BYTES" "$MAX_BYTES" <<'PY'
import json, sys
from datetime import datetime, timezone, timedelta
items = json.load(open(sys.argv[1]))
max_hours = float(sys.argv[2])
min_bytes = int(sys.argv[3])
max_bytes = int(sys.argv[4])
best = None
for it in items:
    if it.get("IsDir"):
        continue
    name = it.get("Name") or it.get("Path") or ""
    if not name.endswith(".age") or name.endswith(".sha256"):
        continue
    mod = it.get("ModTime") or ""
    size = int(it.get("Size") or 0)
    if best is None or mod > best[0]:
        best = (mod, name, size)
if best is None:
    sys.exit(2)
mod, name, size = best
stamp = mod.replace("Z", "+00:00")
when = datetime.fromisoformat(stamp)
if when.tzinfo is None:
    when = when.replace(tzinfo=timezone.utc)
age = datetime.now(timezone.utc) - when
if age > timedelta(hours=max_hours) or age < timedelta(minutes=-15):
    print("age-window", file=sys.stderr)
    sys.exit(3)
if size < min_bytes or size > max_bytes:
    print("size", file=sys.stderr)
    sys.exit(4)
print(name)
print(size)
print(mod)
PY
}

check_prefix() {
  local prefix="$1" parsed name size mod dest sidecar got rc
  local tmp
  tmp="$(mktemp -d)"
  backup_rclone lsjson "${REMOTE}/${prefix}/" >"${tmp}/listing.json"
  set +e
  parsed="$(newest_age "${tmp}/listing.json" 2>"${tmp}/why")"
  rc=$?
  set -e
  if [[ "$rc" -ne 0 ]]; then
    backup_log "FAIL ${prefix}: listing check rc=${rc} $(tr '\n' ' ' <"${tmp}/why")"
    rm -rf "$tmp"
    return 1
  fi
  name="$(printf '%s\n' "$parsed" | sed -n '1p')"
  size="$(printf '%s\n' "$parsed" | sed -n '2p')"
  mod="$(printf '%s\n' "$parsed" | sed -n '3p')"
  dest="${tmp}/${name}"
  sidecar="${dest}.sha256"
  backup_rclone copyto "${REMOTE}/${prefix}/${name}" "$dest"
  backup_rclone copyto "${REMOTE}/${prefix}/${name}.sha256" "$sidecar"
  got="$(stat -c '%s' "$dest")"
  if [[ "$got" != "$size" ]]; then
    backup_log "FAIL ${prefix}: downloaded ${got} bytes, listing said ${size}"
    rm -rf "$tmp"
    return 1
  fi
  if ! (cd "$tmp" && sha256sum -c "$(basename "$sidecar")"); then
    backup_log "FAIL ${prefix}: sha256 mismatch for ${name}"
    rm -rf "$tmp"
    return 1
  fi
  if ! backup_age_header_ok "$dest"; then
    backup_log "FAIL ${prefix}: age header is not valid (${name})"
    rm -rf "$tmp"
    return 1
  fi
  backup_log "OK ${prefix}/${name} size=${size} mod=${mod}"
  rm -rf "$tmp"
  return 0
}

verify_main() {
  local prefix fail=0 lock
  [[ "$MAX_AGE_HOURS" =~ ^[0-9]+$ ]] || backup_die "TELLOMI_BACKUP_MAX_AGE_HOURS must be an integer"
  [[ "$MIN_BYTES" =~ ^[0-9]+$ ]] || backup_die "TELLOMI_BACKUP_MIN_BYTES must be an integer"
  [[ "$MAX_BYTES" =~ ^[0-9]+$ ]] || backup_die "TELLOMI_BACKUP_MAX_BYTES must be an integer"
  require_tools
  backup_load_r2
  if [[ -w /run ]]; then
    lock=/run/tellomi-backup-verify.lock
  else
    lock="${TMPDIR:-/tmp}/tellomi-backup-verify.lock"
  fi
  exec 9>"$lock"
  flock -n 9 || backup_die "another verify holds ${lock}"
  for prefix in mysql etcd redis; do
    if ! check_prefix "$prefix"; then
      fail=1
    fi
  done
  if [[ "$fail" -ne 0 ]]; then
    backup_die "one or more prefixes failed"
  fi
  backup_log "done mysql etcd redis within ${MAX_AGE_HOURS}h"
}

if [[ "$SELFTEST" -eq 1 ]]; then
  self_test
  exit 0
fi
verify_main
