#!/usr/bin/env bash
# Pull the pinned images the reliability suite starts, or load a tarball
# restored by actions/cache. A miss rewrites the tarball so the cache save
# at the end of the job can keep it.
set -euo pipefail

cache="${RELIABILITY_IMAGE_CACHE:-/tmp/reliability-images}"
mkdir -p "$cache"
tar="$cache/images.tar"

images=(
  mysql:8.0.40
  redis:7.4.2-alpine
  quay.io/coreos/etcd:v3.5.11
  apache/kafka:3.9.0
  docker.io/pgsty/silo:RELEASE.2026-10-26T00-00-00Z
  testcontainers/ryuk:0.11.0
)

if [ -f "$tar" ]; then
  docker load -i "$tar"
fi

missing=0
for image in "${images[@]}"; do
  if ! docker image inspect "$image" >/dev/null 2>&1; then
    docker pull "$image"
    missing=1
  fi
done

if [ "$missing" -eq 1 ] || [ ! -f "$tar" ]; then
  docker save -o "$tar" "${images[@]}"
fi
