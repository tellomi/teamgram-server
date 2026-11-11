#!/bin/bash
# Recreate tellomi-main-teamgram from a saved inspect. Not compose.
# Paths are fixed. The backup container name is fixed.
# Does not print env values. Does not FLUSHALL.
set -euo pipefail

NAME=tellomi-main-teamgram
BAK=tellomi-main-teamgram.dc-bak
INSPECT=/var/backups/tellomi-main-teamgram.inspect.json
CREATE=/var/backups/tellomi-main-teamgram.create.json
SCRIPT=/var/backups/recreate_teamgram_from_inspect.py
DC_OPTIONS=1:8.137.198.240:10443,1:8.137.198.240:443,2:8.137.198.240:10443,2:8.137.198.240:443
PUBLIC_HOST=8.137.198.240
TXT_DOMAIN=-

usage() {
  echo "usage: $0 apply|rollback" >&2
  exit 2
}

if [[ $# -ne 1 ]]; then
  usage
fi

install -d /var/backups

if [[ "$1" == "rollback" ]]; then
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
python3 "$SCRIPT" "$INSPECT" "$CREATE" "$PUBLIC_HOST" "$DC_OPTIONS" "$TXT_DOMAIN"

docker stop "$NAME"
docker rename "$NAME" "$BAK"
if ! python3 "$SCRIPT" create "$CREATE"; then
  docker rename "$BAK" "$NAME"
  docker start "$NAME"
  echo "docker create failed; restored ${NAME} from ${BAK}" >&2
  exit 1
fi
if ! docker start "$NAME"; then
  docker rm -f "$NAME"
  docker rename "$BAK" "$NAME"
  docker start "$NAME"
  echo "docker start failed; restored ${NAME} from ${BAK}" >&2
  exit 1
fi
sleep 2
if [[ "$(docker inspect -f '{{.State.Running}}' "$NAME")" != "true" ]]; then
  docker rm -f "$NAME"
  docker rename "$BAK" "$NAME"
  docker start "$NAME"
  echo "new container exited; restored ${NAME} from ${BAK}" >&2
  exit 1
fi
