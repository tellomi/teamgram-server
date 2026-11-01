#!/usr/bin/env bash
# Pull the published image and recreate the Chengdu stack. Do not build.
# Always merges docker-compose.geoip.yaml so the DB-IP Lite mount is reapplied.
# A plain `docker compose up` drops that volume.
set -euo pipefail

cd "$(dirname "$0")"

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
