#!/usr/bin/env bash
# Pinned production deploy. Pulls published images; never builds.
#
#   /opt/tellomi/deploy-pull.sh <7-char short SHA>
#   TELLOMI_DEPLOY_CHECK_ONLY=1 /opt/tellomi/deploy-pull.sh <sha>
#
# The production copy lives at /opt/tellomi/deploy-pull.sh, outside the
# checkout, so the checkout path is TELLOMI_DEPLOY_DIR (default
# /opt/tellomi/teamgram-server), not the script's directory. Copy this file
# and scripts/ops onto the host; git checkout does not update them
# (docs/SECURITY-OPS.md). git fetch below uses the public GitHub remote. A
# private repo needs a read-only deploy key on the host first. Do not fetch
# through a mirror.
#
# Disk: if / has less than TELLOMI_MIN_FREE_GB (default 10) GiB free, old
# tellomi image tags are removed before checkout. Still short: exit 3, and
# the checkout, .env, and containers are left alone. CHECK_ONLY stops before
# checkout; when space is already short, cleanup is a dry-run and the script
# exits 3 because a dry-run does not free space. After a successful deploy,
# cleanup runs again.
#
# Steps: check out that commit, pin TELLOMI_IMAGE_TAG in .env, pull the
# teamgram image (and asr when the commit defines it), run mysql-migrate,
# then recreate asr (waiting for its healthcheck) and teamgram only.
# Kafka, MySQL, Redis, etcd, MinIO, coturn and mediamtx are left running.
#
# The image must already be in ACR: the "Publish images" workflow on master
# tags it with the same 7-character SHA. Back up the database to
# /opt/tellomi/backups (chmod 600) before a deploy that adds migrations.
set -euo pipefail

SHA="${1:-}"
if [[ ! "$SHA" =~ ^[0-9a-f]{7}$ ]]; then
  echo "usage: $0 <7-char short SHA>" >&2
  exit 2
fi

MIN_FREE_GB="${TELLOMI_MIN_FREE_GB:-10}"
CLEANUP="${TELLOMI_IMAGE_CLEANUP:-/usr/local/bin/tellomi-image-cleanup.sh}"
if [[ ! "$MIN_FREE_GB" =~ ^[0-9]+$ ]]; then
  echo "TELLOMI_MIN_FREE_GB must be an integer" >&2
  exit 2
fi

avail_gb() {
  df -BG --output=avail / | tail -1 | tr -dc '0-9'
}

# Runs before checkout. A short disk must not rewrite .env or recreate containers.
disk_precheck() {
  local free mode
  local -a dry_args
  free="$(avail_gb)"
  if [[ -z "$free" ]]; then
    echo "disk: could not read free space on /" >&2
    exit 1
  fi
  echo "disk: ${free}G free on / (need ${MIN_FREE_GB}G)"
  if (( free < MIN_FREE_GB )); then
    dry_args=()
    mode="cleanup"
    if [[ "${TELLOMI_DEPLOY_CHECK_ONLY:-}" == "1" ]]; then
      dry_args=(--dry-run)
      mode="dry-run"
    fi
    if [[ ! -x "$CLEANUP" ]]; then
      echo "ABORT: only ${free}G free on / and ${CLEANUP} is not executable. Nothing deployed; containers untouched." >&2
      exit 3
    fi
    echo "disk: below ${MIN_FREE_GB}G, cleaning old images (${mode})"
    "$CLEANUP" "${dry_args[@]}" --keep "$SHA"
    free="$(avail_gb)"
    if (( free < MIN_FREE_GB )); then
      echo "ABORT: only ${free}G free on / after cleanup (need ${MIN_FREE_GB}G). Nothing deployed; containers untouched." >&2
      logger -t tellomi-deploy -p user.err "deploy ${SHA} aborted: ${free}G free on /" 2>/dev/null || true
      exit 3
    fi
  fi
  echo "disk: ok"
}

disk_precheck
if [[ "${TELLOMI_DEPLOY_CHECK_ONLY:-}" == "1" ]]; then
  echo "CHECK_ONLY: stop before deploy"
  exit 0
fi

DIR="${TELLOMI_DEPLOY_DIR:-/opt/tellomi/teamgram-server}"
cd "$DIR"

timeout 120 git fetch -q origin master || echo "warn: git fetch failed; using local objects" >&2
git checkout -q --detach "$SHA"

touch .env
if grep -q '^TELLOMI_IMAGE_TAG=' .env; then
  sed -i "s/^TELLOMI_IMAGE_TAG=.*/TELLOMI_IMAGE_TAG=$SHA/" .env
else
  echo "TELLOMI_IMAGE_TAG=$SHA" >> .env
fi
export TELLOMI_IMAGE_TAG="$SHA"
echo "TELLOMI_IMAGE_TAG=${TELLOMI_IMAGE_TAG}"

# docker-compose.geoip.yaml bind-mounts the DB-IP Lite MMDB. Without the file
# compose would mount a missing path, so stop here instead.
if [[ -z "${TELLOMI_GEOIP_MMDB_HOST:-}" ]]; then
  TELLOMI_GEOIP_MMDB_HOST="$(grep -E '^[[:space:]]*TELLOMI_GEOIP_MMDB_HOST=' .env | tail -1 | cut -d= -f2- | tr -d '"' | tr -d "'" | xargs || true)"
fi
host_file="${TELLOMI_GEOIP_MMDB_HOST:-./data/geoip/dbip-city-lite.mmdb}"
if [[ ! -f "$host_file" ]]; then
  echo "GeoIP database not found at ${host_file}. Set TELLOMI_GEOIP_MMDB_HOST in .env." >&2
  exit 1
fi

# docker-compose.tellomi.yaml is the host-only override (loopback ports,
# coturn config, secrets from .env). It is not in git. Without it the
# infrastructure ports would be published on the public address.
if [[ ! -f docker-compose.tellomi.yaml ]]; then
  echo "docker-compose.tellomi.yaml is missing in ${DIR}. Refusing to deploy without the host override." >&2
  exit 1
fi

# Project "tellomi" is the running stack (tellomi-teamgram-1, tellomi-asr-1).
# Keep -f docker-compose.geoip.yaml in the same command as up, or the mount drops.
files=(-f docker-compose-env.yaml -f docker-compose.yaml -f docker-compose.geoip.yaml -f docker-compose.tellomi.yaml)
C=(docker compose -p tellomi "${files[@]}")

"${C[@]}" config --quiet
if ! "${C[@]}" config --images | grep -q "/teamgram:${SHA}$"; then
  echo "compose does not resolve the teamgram image to tag ${SHA}" >&2
  exit 1
fi

apps=(teamgram)
if "${C[@]}" config --services | grep -qx asr; then
  apps=(asr teamgram)
fi

echo "pulling ${apps[*]}"
"${C[@]}" pull "${apps[@]}"

echo "applying migrations"
"${C[@]}" up --no-deps mysql-migrate
code="$(docker inspect -f '{{.State.ExitCode}}' mysql-migrate 2>/dev/null || echo 1)"
if [[ "$code" != "0" ]]; then
  echo "mysql-migrate exited ${code}; teamgram was not recreated" >&2
  docker logs --tail 50 mysql-migrate >&2 || true
  exit 1
fi

if [[ " ${apps[*]} " == *" asr "* ]]; then
  echo "recreating asr"
  "${C[@]}" up -d --no-deps --wait --wait-timeout 300 asr
fi
echo "recreating teamgram"
"${C[@]}" up -d --no-deps teamgram

docker ps --format '{{.Names}} {{.Image}} {{.Status}}' | grep -E 'tellomi-(teamgram|asr)-1' || true
echo "DEPLOY_DONE ${SHA}"
if [[ -x "$CLEANUP" ]]; then
  "$CLEANUP" --keep "$SHA" || echo "warn: post-deploy image cleanup failed" >&2
else
  echo "warn: ${CLEANUP} missing; skipped post-deploy image cleanup" >&2
fi
