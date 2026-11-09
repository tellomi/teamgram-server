#!/bin/bash
# Local N05/N06 check. Talks only to a loopback MTProto port and the
# synthetic numbers in e2e/client/config.json. It does not read production
# credentials and it does not change server config.
set -eu
addr="${TELLOMI_E2E_ADDR:-127.0.0.1:10443}"
host="${addr%%:*}"
case "$host" in
  127.0.0.1|localhost|::1) ;;
  *)
    echo "refusing non-loopback address" >&2
    exit 2
    ;;
esac
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root/e2e/client"
export TELLOMI_E2E=1
export TELLOMI_E2E_ADDR="$addr"
export TELLOMI_E2E_LOCAL_RESET=1
exec go test -count=1 -timeout 20m -v \
  -run 'TestLoginNoticeFromServiceUser|TestQRLoginNoticeWithoutPassword|TestQRLoginNoticeWithPassword|TestLocalAgedResetDropsOtherSessions|TestFreshLoginCannotResetOlderSession' \
  .
