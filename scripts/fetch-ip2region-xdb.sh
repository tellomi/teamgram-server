#!/usr/bin/env bash
# Download ip2region v4 and v6 xdb from the official v3.18.0 tag and check SHA256.
# The image build and the tests both run this script. See docs/ip2region.md.
set -euo pipefail

dest="${1:-/app/data/ip2region}"
tag="v3.18.0"
base="https://github.com/lionsoul2014/ip2region/raw/refs/tags/${tag}/data"

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -n "${IP2REGION_SHA256SUMS:-}" ]]; then
  sums="$IP2REGION_SHA256SUMS"
elif [[ -f "$script_dir/../deploy/ip2region/SHA256SUMS" ]]; then
  sums="$script_dir/../deploy/ip2region/SHA256SUMS"
elif [[ -f "$script_dir/SHA256SUMS" ]]; then
  sums="$script_dir/SHA256SUMS"
else
  echo "ip2region SHA256SUMS not found" >&2
  exit 1
fi
sums="$(cd "$(dirname "$sums")" && pwd)/$(basename "$sums")"

mkdir -p "$dest"
exec 9>"$dest/.fetch.lock"
if command -v flock >/dev/null 2>&1; then
  flock 9
fi

if [[ -f "$dest/ip2region_v4.xdb" && -f "$dest/ip2region_v6.xdb" ]]; then
  if (cd "$dest" && sha256sum -c "$sums" >/dev/null); then
    echo "ip2region xdb checksum ok in $dest"
    exit 0
  fi
  echo "ip2region xdb checksum mismatch in $dest; downloading again" >&2
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

for name in ip2region_v4.xdb ip2region_v6.xdb; do
  echo "downloading $name from $base/$name"
  curl -fsSL --retry 5 --retry-delay 2 --connect-timeout 20 --max-time 300 \
    -o "$tmp/$name" "$base/$name"
done

(cd "$tmp" && sha256sum -c "$sums")
mv -f "$tmp/ip2region_v4.xdb" "$dest/ip2region_v4.xdb"
mv -f "$tmp/ip2region_v6.xdb" "$dest/ip2region_v6.xdb"
chmod 0644 "$dest/ip2region_v4.xdb" "$dest/ip2region_v6.xdb"
echo "ip2region xdb verified into $dest"
