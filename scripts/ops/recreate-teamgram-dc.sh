#!/bin/bash
# Recreate tellomi-main-teamgram from a saved inspect. Not compose.
# Paths and the backup container name are fixed.
# Does not print env values. Does not FLUSHALL.
set -euo pipefail
umask 077

NAME=tellomi-main-teamgram
BAK=tellomi-main-teamgram.dc-bak
INSPECT=/var/backups/tellomi-main-teamgram.inspect.json
CREATE=/var/backups/tellomi-main-teamgram.create.json
SCRIPT=/var/backups/recreate_teamgram_from_inspect.py
READCFG=/var/backups/help-getconfig-dc.py
DC_OPTIONS=1:8.137.198.240:10443,1:8.137.198.240:443,2:8.137.198.240:10443,2:8.137.198.240:443
PUBLIC_HOST=8.137.198.240
TXT_DOMAIN=-

usage() {
  echo "usage: $0 apply|rollback" >&2
  exit 2
}

backup_usable() {
  docker inspect "$BAK" >/dev/null 2>&1 || return 1
  local image status
  image="$(docker inspect -f '{{.Config.Image}}' "$BAK" 2>/dev/null || true)"
  status="$(docker inspect -f '{{.State.Status}}' "$BAK" 2>/dev/null || true)"
  [[ -n "$image" ]] || return 1
  case "$status" in
    created|running|exited|paused) return 0 ;;
    *) return 1 ;;
  esac
}

restore_backup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
  docker rename "$BAK" "$NAME"
  docker start "$NAME"
}

expect_config() {
  local out
  out="$(python3 "$READCFG" --host 127.0.0.1 --port 10443 --container "$NAME" --workdir /app/bin --timeout 8)" || return 1
  python3 -c '
import sys
want = {
    "1 8.137.198.240 10443 ipv6=0",
    "1 8.137.198.240 443 ipv6=0",
    "2 8.137.198.240 10443 ipv6=0",
    "2 8.137.198.240 443 ipv6=0",
}
got = {line.strip() for line in sys.stdin.read().splitlines() if line.strip()}
sys.exit(0 if got == want else 1)
' <<<"$out"
}

if [[ $# -ne 1 ]]; then
  usage
fi

install -d /var/backups

if [[ "$1" == "rollback" ]]; then
  if ! backup_usable; then
    echo "backup container ${BAK} is missing or not usable; production left unchanged" >&2
    exit 1
  fi
  docker stop "$NAME"
  docker rename "$NAME" "${NAME}.rollback-hold"
  docker rename "$BAK" "$NAME"
  docker start "$NAME"
  docker rm -f "${NAME}.rollback-hold"
  exit 0
fi

if [[ "$1" != "apply" ]]; then
  usage
fi

if docker inspect "$BAK" >/dev/null 2>&1; then
  echo "backup container ${BAK} already exists; not overwriting it" >&2
  exit 1
fi

docker inspect "$NAME" > "$INSPECT"
chmod 0600 "$INSPECT"
python3 "$SCRIPT" "$INSPECT" "$CREATE" "$PUBLIC_HOST" "$DC_OPTIONS" "$TXT_DOMAIN"
chmod 0600 "$CREATE"

docker stop "$NAME"
docker rename "$NAME" "$BAK"
if ! python3 "$SCRIPT" create "$CREATE"; then
  restore_backup
  echo "docker create failed; restored ${NAME} from ${BAK}" >&2
  exit 1
fi
if ! docker start "$NAME"; then
  restore_backup
  echo "docker start failed; restored ${NAME} from ${BAK}" >&2
  exit 1
fi

ok=0
for _ in 1 2 3 4 5 6; do
  if expect_config; then
    ok=1
    break
  fi
  sleep 5
done
if [[ "$ok" -ne 1 ]]; then
  restore_backup
  echo "help.getConfig did not return the four relay dc_options; restored ${NAME} from ${BAK}" >&2
  exit 1
fi
