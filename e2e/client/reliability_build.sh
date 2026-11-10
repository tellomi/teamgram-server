#!/usr/bin/env bash
# Build the teamgram processes the reliability suite runs.
# idgen, status, authsession, media, biz, sync, session, and gnetway are pure
# Go and build while libvips installs. msg, bff, and dfs import cgo image
# libraries (webp, and libvips for dfs) and wait for that install.
# CI caches the Go build cache around this script.
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

build_one() {
  local name="$1" pkg="$2" cgo="$3"
  echo "build ${name}"
  CGO_ENABLED="$cgo" go build -o "teamgramd/bin/${name}" "./${pkg}"
}

# name, package. These do not import cgo.
others=(
  "idgen app/service/idgen/cmd/idgen"
  "status app/service/status/cmd/status"
  "authsession app/service/authsession/cmd/authsession"
  "media app/service/media/cmd/media"
  "biz app/service/biz/biz/cmd/biz"
  "sync app/messenger/sync/cmd/sync"
  "session app/interface/session/cmd/session"
  "gnetway app/interface/gnetway/cmd/gnetway"
)

# Four big compiles at once crowd the machine. Keep a small pool.
max=3
pids=()
for spec in "${others[@]}"; do
  name="${spec%% *}"
  pkg="${spec#* }"
  build_one "$name" "$pkg" 0 &
  pids+=("$!")
  if [ "${#pids[@]}" -ge "$max" ]; then
    if ! wait "${pids[0]}"; then
      exit 1
    fi
    pids=("${pids[@]:1}")
  fi
done
for pid in "${pids[@]}"; do
  if ! wait "$pid"; then
    exit 1
  fi
done

if [ -n "$apt_pid" ]; then
  wait "$apt_pid"
fi
# webp (msg, bff) and libvips (dfs). dfs is the long compile, so it starts
# first and overlaps the other two.
fail=0
build_one dfs app/service/dfs/cmd/dfs 1 &
dfs_pid=$!
build_one msg app/messenger/msg/cmd/msg 1 &
msg_pid=$!
wait "$msg_pid" || fail=1
build_one bff app/bff/bff/cmd/bff 1 &
bff_pid=$!
wait "$bff_pid" || fail=1
wait "$dfs_pid" || fail=1
if [ "$fail" -ne 0 ]; then
  exit 1
fi
