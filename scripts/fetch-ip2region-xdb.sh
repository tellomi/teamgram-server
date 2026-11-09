#!/usr/bin/env bash
# Download the pinned ip2region v4 and v6 xdb and check SHA256.
# The image build and the tests both run this script. See docs/ip2region.md.
#
# commit is the v3.18.0 tag (c1a1fc7d5941760db3f8431dc05c48cf7f0e30a1).
# A full GET of ip2region_v6.xdb from GitHub raw returns 504, so that URL is
# read with Range requests. Each request is retried with exponential backoff.
# If GitHub raw still fails, the same commit is fetched from the jsDelivr
# GitHub CDN. jsDelivr refuses a file over 20MB, so the v6 database has to
# come from GitHub raw. A checksum mismatch fails the script.
set -euo pipefail

dest="${1:-/app/data/ip2region}"
commit="c1a1fc7d5941760db3f8431dc05c48cf7f0e30a1"
raw_base="https://raw.githubusercontent.com/lionsoul2014/ip2region/${commit}/data"
cdn_base="https://cdn.jsdelivr.net/gh/lionsoul2014/ip2region@${commit}/data"
chunk_size=8388608
max_attempts=5

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

expected_sha() {
  local name="$1" want
  want="$(awk -v name="$name" '$2 == name { print $1; exit }' "$sums")"
  if [[ -z "$want" ]]; then
    echo "ip2region: no sha256 for $name in $sums" >&2
    return 1
  fi
  printf '%s' "$want"
}

file_sha_ok() {
  local name="$1" path="$2" want got
  want="$(expected_sha "$name")"
  got="$(sha256sum "$path" | awk '{ print $1 }')"
  if [[ "$got" != "$want" ]]; then
    echo "ip2region: sha256 mismatch for $name" >&2
    return 1
  fi
}

content_range_total() {
  local hdr="$1"
  tr -d '\r' < "$hdr" | awk '
    BEGIN { IGNORECASE = 1 }
    /^content-range:/ { n = $3 }
    END {
      split(n, a, "/")
      print a[2]
    }
  '
}

retry_sleep() {
  local attempt="$1" delay="$2"
  echo "ip2region: attempt ${attempt}/${max_attempts} failed, retrying in ${delay}s" >&2
  sleep "$delay"
}

# Print the HTTP status of a one-byte range probe. Transient statuses are
# retried. 206 means the server advertised a total size.
probe_range() {
  local url="$1" hdr="$2" attempt=1 delay=2 code
  while true; do
    code="$(curl -sS -L -D "$hdr" -o /dev/null -r 0-0 -w '%{http_code}' \
      --connect-timeout 20 --max-time 60 "$url" || true)"
    case "$code" in
      206|200|403|404)
        printf '%s' "$code"
        return 0
        ;;
    esac
    if [[ "$attempt" -ge "$max_attempts" ]]; then
      printf '%s' "${code:-000}"
      return 0
    fi
    retry_sleep "$attempt" "$delay"
    delay=$((delay * 2))
    attempt=$((attempt + 1))
  done
}

download_chunk() {
  local url="$1" part="$2" start="$3" end="$4"
  local attempt=1 delay=2 got expect
  expect=$((end - start + 1))
  while true; do
    if curl -fsSL --connect-timeout 20 --max-time 120 \
      -r "${start}-${end}" -o "$part" "$url"; then
      got="$(wc -c < "$part" | tr -d ' ')"
      if [[ "$got" -eq "$expect" ]]; then
        return 0
      fi
      echo "ip2region: short range ${start}-${end}: got ${got}, want ${expect}" >&2
    fi
    rm -f "$part"
    if [[ "$attempt" -ge "$max_attempts" ]]; then
      return 1
    fi
    retry_sleep "$attempt" "$delay"
    delay=$((delay * 2))
    attempt=$((attempt + 1))
  done
}

download_ranged() {
  local url="$1" dest="$2" total="$3"
  local start=0 end part out size
  part="$(mktemp)"
  out="$(mktemp)"
  : > "$out"
  while [[ "$start" -lt "$total" ]]; do
    end=$((start + chunk_size - 1))
    if [[ "$end" -ge "$total" ]]; then
      end=$((total - 1))
    fi
    if ! download_chunk "$url" "$part" "$start" "$end"; then
      rm -f "$part" "$out"
      return 1
    fi
    cat "$part" >> "$out"
    start=$((end + 1))
  done
  rm -f "$part"
  size="$(wc -c < "$out" | tr -d ' ')"
  if [[ "$size" -ne "$total" ]]; then
    echo "ip2region: assembled ${size} bytes, content-range said ${total}" >&2
    rm -f "$out"
    return 1
  fi
  mv -f "$out" "$dest"
}

download_full() {
  local url="$1" dest="$2" attempt=1 delay=2
  while true; do
    if curl -fsSL --connect-timeout 20 --max-time 300 -o "$dest" "$url"; then
      return 0
    fi
    rm -f "$dest"
    if [[ "$attempt" -ge "$max_attempts" ]]; then
      return 1
    fi
    retry_sleep "$attempt" "$delay"
    delay=$((delay * 2))
    attempt=$((attempt + 1))
  done
}

download_url() {
  local url="$1" dest="$2" hdr code total
  hdr="$(mktemp)"
  code="$(probe_range "$url" "$hdr")"
  if [[ "$code" == "206" ]]; then
    total="$(content_range_total "$hdr")"
    rm -f "$hdr"
    if [[ -n "$total" && "$total" != "*" ]] && download_ranged "$url" "$dest" "$total"; then
      return 0
    fi
  else
    rm -f "$hdr"
  fi
  if [[ "$code" == "403" || "$code" == "404" ]]; then
    echo "ip2region: HTTP ${code} from $url" >&2
    return 1
  fi
  download_full "$url" "$dest"
}

fetch_one() {
  local name="$1" raw_url cdn_url
  raw_url="${raw_base}/${name}"
  cdn_url="${cdn_base}/${name}"
  echo "downloading $name from $raw_url"
  rm -f "$tmp/$name"
  if download_url "$raw_url" "$tmp/$name" && file_sha_ok "$name" "$tmp/$name"; then
    echo "sha256 ok for $name"
    return 0
  fi
  echo "ip2region: GitHub raw failed for $name; trying jsDelivr $cdn_url" >&2
  rm -f "$tmp/$name"
  if download_url "$cdn_url" "$tmp/$name" && file_sha_ok "$name" "$tmp/$name"; then
    echo "sha256 ok for $name from jsDelivr"
    return 0
  fi
  rm -f "$tmp/$name"
  echo "ip2region: could not download $name" >&2
  return 1
}

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

fetch_one ip2region_v4.xdb
fetch_one ip2region_v6.xdb

(cd "$tmp" && sha256sum -c "$sums")
mv -f "$tmp/ip2region_v4.xdb" "$dest/ip2region_v4.xdb"
mv -f "$tmp/ip2region_v6.xdb" "$dest/ip2region_v6.xdb"
chmod 0644 "$dest/ip2region_v4.xdb" "$dest/ip2region_v6.xdb"
echo "ip2region xdb verified into $dest"
