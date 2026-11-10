#!/usr/bin/env bash
# Build the teamgram processes the reliability suite runs, and the test binary.
# The pure-Go services share one compile. msg, bff, and dfs import cgo image
# libraries (webp, and libvips for dfs) and build after that install. The
# reliability test binary compiles during the cgo build. CI caches the Go
# build cache around this script.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"
mkdir -p teamgramd/bin

if ! pkg-config --exists vips 2>/dev/null; then
  sudo apt-get update -qq
  sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends gcc pkg-config libvips-dev &
  apt_pid=$!
else
  apt_pid=""
fi

echo "build pure-go services"
CGO_ENABLED=0 go build -o teamgramd/bin/ \
  ./app/service/idgen/cmd/idgen \
  ./app/service/status/cmd/status \
  ./app/service/authsession/cmd/authsession \
  ./app/service/media/cmd/media \
  ./app/service/biz/biz/cmd/biz \
  ./app/messenger/sync/cmd/sync \
  ./app/interface/session/cmd/session \
  ./app/interface/gnetway/cmd/gnetway

if [ -n "$apt_pid" ]; then
  wait "$apt_pid"
fi

echo "build cgo services"
CGO_ENABLED=1 go build -o teamgramd/bin/ \
  ./app/messenger/msg/cmd/msg \
  ./app/bff/bff/cmd/bff \
  ./app/service/dfs/cmd/dfs &
cgo_pid=$!

echo "build reliability test"
(
  cd "$root/e2e/client"
  go test -tags reliability -c -o "$root/teamgramd/bin/reliability.test" .
) &
test_pid=$!

fail=0
wait "$cgo_pid" || fail=1
wait "$test_pid" || fail=1
exit "$fail"
