#!/usr/bin/env bash
# cron (every 10 min): if / is over TELLOMI_DISK_WARN_PCT (default 85) percent
# used, warn in syslog and remove old tellomi image tags. The cleanup script
# keeps in-use tags, --keep tags, and the 2 newest other teamgram tags.
set -euo pipefail

THRESHOLD="${TELLOMI_DISK_WARN_PCT:-85}"
CLEANUP="${TELLOMI_IMAGE_CLEANUP:-/usr/local/bin/tellomi-image-cleanup.sh}"

if [[ ! "$THRESHOLD" =~ ^[0-9]+$ ]]; then
  logger -t tellomi-disk-watch -p user.err "TELLOMI_DISK_WARN_PCT must be an integer" 2>/dev/null || true
  exit 0
fi

P="$(df --output=pcent / | tail -1 | tr -dc '0-9')"
if [[ -z "$P" ]]; then
  logger -t tellomi-disk-watch -p user.err "could not read disk percent for /" 2>/dev/null || true
  exit 0
fi

if (( P > THRESHOLD )); then
  logger -t tellomi-disk-watch -p user.warning "root disk ${P}% used (>${THRESHOLD}%), running image cleanup" 2>/dev/null || true
  if [[ -x "$CLEANUP" ]]; then
    "$CLEANUP" >/dev/null 2>&1 || true
  else
    logger -t tellomi-disk-watch -p user.err "missing ${CLEANUP}" 2>/dev/null || true
  fi
  after="$(df --output=pcent / | tail -1 | tr -dc '0-9')"
  logger -t tellomi-disk-watch "after cleanup root disk ${after}% used" 2>/dev/null || true
fi
exit 0
