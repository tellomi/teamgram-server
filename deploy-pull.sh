#!/usr/bin/env bash
# Pull the published image and recreate the Chengdu stack. Do not build.
# Always merges docker-compose.geoip.yaml so the DB-IP Lite mount is reapplied.
# A plain `docker compose up` drops that volume.
set -euo pipefail

cd "$(dirname "$0")"

# Pin the app image to the 7-character commit SHA. "latest" moves, and an
# empty or "local" tag must not pull it. A shell value wins over .env; an
# explicit 7-character SHA (a rollback) is kept.
image_tag="${TELLOMI_IMAGE_TAG:-}"
if [[ -z "$image_tag" && -f .env ]]; then
  image_tag="$(grep -E '^[[:space:]]*TELLOMI_IMAGE_TAG=' .env | tail -1 | cut -d= -f2- | tr -d '"' | tr -d "'")"
  image_tag="${image_tag#"${image_tag%%[![:space:]]*}"}"
  image_tag="${image_tag%"${image_tag##*[![:space:]]}"}"
fi
case "$image_tag" in
  ""|latest|local)
    if ! image_tag="$(git rev-parse --short=7 HEAD 2>/dev/null)"; then
      echo "Set TELLOMI_IMAGE_TAG to the 7-character commit SHA. latest is not used, and this directory is not a git checkout." >&2
      exit 1
    fi
    ;;
esac
if [[ ! "$image_tag" =~ ^[0-9a-f]{7}$ ]]; then
  echo "TELLOMI_IMAGE_TAG=${image_tag} is not a 7-character commit SHA. Refusing to pull latest." >&2
  exit 1
fi
export TELLOMI_IMAGE_TAG="$image_tag"
echo "TELLOMI_IMAGE_TAG=${TELLOMI_IMAGE_TAG}"

if [[ -z "${TELLOMI_GEOIP_MMDB_HOST:-}" && -f .env ]]; then
  TELLOMI_GEOIP_MMDB_HOST="$(grep -E '^[[:space:]]*TELLOMI_GEOIP_MMDB_HOST=' .env | tail -1 | cut -d= -f2- | tr -d '"' | tr -d "'")"
  TELLOMI_GEOIP_MMDB_HOST="${TELLOMI_GEOIP_MMDB_HOST#"${TELLOMI_GEOIP_MMDB_HOST%%[![:space:]]*}"}"
  TELLOMI_GEOIP_MMDB_HOST="${TELLOMI_GEOIP_MMDB_HOST%"${TELLOMI_GEOIP_MMDB_HOST##*[![:space:]]}"}"
fi
host_file="${TELLOMI_GEOIP_MMDB_HOST:-./data/geoip/dbip-city-lite.mmdb}"
if [[ ! -f "$host_file" ]]; then
  echo "GeoIP database not found at ${host_file}." >&2
  echo "Set TELLOMI_GEOIP_MMDB_HOST to the DB-IP Lite city .mmdb on this host." >&2
  echo "Refusing to continue: compose would bind-mount a missing path and drop the database." >&2
  exit 1
fi

# One project: env + app + GeoIP. MinIO stays out (profile local-minio).
# teamgram depends_on kafka, which is defined in docker-compose-env.yaml, so
# the app files cannot be composed alone. GeoIP is always in the same command
# so `up` cannot drop the mount.
files=(-f docker-compose-env.yaml -f docker-compose.yaml -f docker-compose.geoip.yaml)

echo "pulling app images"
docker compose "${files[@]}" pull teamgram coturn mediamtx

echo "recreating the stack with the GeoIP mount"
docker compose "${files[@]}" up -d
