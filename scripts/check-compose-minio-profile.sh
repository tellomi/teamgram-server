#!/usr/bin/env bash
# Default `docker compose -f docker-compose-env.yaml config` must not include
# minio or minio-mc. Those services are profile local-minio. A full `up`
# without the profile must not pull them.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

if ! command -v docker >/dev/null 2>&1 || ! docker compose version >/dev/null 2>&1; then
  echo "docker compose is required" >&2
  exit 1
fi

# Do not let a developer .env or the shell hide the pinned default image.
export COMPOSE_DISABLE_ENV_FILE=1
unset MINIO_IMAGE MINIO_MC_IMAGE || true

fail() {
  echo "check-compose-minio-profile: $*" >&2
  exit 1
}

services_of() {
  # shellcheck disable=SC2068
  docker compose "$@" config --services
}

default="$(services_of -f docker-compose-env.yaml)"
printf '%s\n' "$default"
for name in minio minio-mc; do
  if printf '%s\n' "$default" | grep -qx "$name"; then
    fail "default config includes ${name}"
  fi
done

# A depends_on toward a profile-only service fails config. Success plus the
# service list above is the check. Also render the project and reject a
# leftover service key.
rendered="$(docker compose -f docker-compose-env.yaml config)"
if printf '%s\n' "$rendered" | grep -Eq '^  (minio|minio-mc):$'; then
  fail "rendered default config still has a minio service"
fi

profiled="$(services_of -f docker-compose-env.yaml --profile local-minio)"
for name in minio minio-mc; do
  if ! printf '%s\n' "$profiled" | grep -qx "$name"; then
    fail "profile local-minio is missing ${name}"
  fi
done

profiled_cfg="$(docker compose -f docker-compose-env.yaml --profile local-minio config)"
grep -Fq "image: docker.io/pgsty/silo:RELEASE.2026-10-26T00-00-00Z" <<<"$profiled_cfg" \
  || fail "minio image is not the pinned pullable tag"
grep -Fq "image: docker.io/pgsty/mc:RELEASE.2026-10-26T00-00-00Z" <<<"$profiled_cfg" \
  || fail "minio-mc image is not the pinned pullable tag"
if grep -F "daocloud.io" <<<"$profiled_cfg" >/dev/null; then
  fail "profile config still references daocloud.io"
fi

combined="$(services_of -f docker-compose-env.yaml -f docker-compose.yaml)"
for name in minio minio-mc; do
  if printf '%s\n' "$combined" | grep -qx "$name"; then
    fail "combined default config includes ${name}"
  fi
done

if ! grep -Fq -- '-f docker-compose.geoip.yaml' deploy-pull.sh; then
  fail "deploy-pull.sh does not merge docker-compose.geoip.yaml"
fi
if ! grep -Fq 'files=(-f docker-compose-env.yaml -f docker-compose.yaml -f docker-compose.geoip.yaml)' deploy-pull.sh; then
  fail "deploy-pull.sh must pass -f docker-compose.geoip.yaml on pull and up"
fi
if ! grep -Fq '"${files[@]}" pull' deploy-pull.sh || ! grep -Fq '"${files[@]}" up' deploy-pull.sh; then
  fail "deploy-pull.sh pull and up must use the geoip file list"
fi
if ! grep -Fq 'pull teamgram coturn mediamtx asr' deploy-pull.sh; then
  fail "deploy-pull.sh must pull the asr image"
fi

geo="$(docker compose -f docker-compose-env.yaml -f docker-compose.yaml -f docker-compose.geoip.yaml config)"
grep -Fq "/app/bin/GeoLite2-City.mmdb" <<<"$geo" \
  || fail "merged config is missing the GeoIP mount"
grep -Fq "TELLOMI_TRANSCRIBE_ENABLED" <<<"$geo" \
  || fail "merged config is missing TELLOMI_TRANSCRIBE_ENABLED"
grep -Eq 'TELLOMI_TRANSCRIBE_ENABLED: "?true"?' <<<"$geo" \
  || fail "TELLOMI_TRANSCRIBE_ENABLED is not true when the asr service is in the stack"
geo_services="$(docker compose -f docker-compose-env.yaml -f docker-compose.yaml -f docker-compose.geoip.yaml config --services)"
for name in minio minio-mc; do
  if printf '%s\n' "$geo_services" | grep -qx "$name"; then
    fail "full deploy config includes ${name}"
  fi
done
if git ls-files '*.mmdb' | grep -q .; then
  fail "an mmdb file is tracked; do not commit the GeoIP database"
fi

echo "ok: default compose services exclude minio; geoip mount is in the deploy"
