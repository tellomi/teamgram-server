#!/usr/bin/env bash
# Regression for staging Compose isolation. Renders config only. Does not up.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
export PATH="${HOME}/.local/bin:${PATH}"

if docker compose version >/dev/null 2>&1; then
  compose=(docker compose)
elif command -v docker-compose >/dev/null 2>&1; then
  compose=(docker-compose)
else
  echo "docker compose is required" >&2
  exit 1
fi

files=(
  -f "$root/docker-compose-env.yaml"
  -f "$root/docker-compose.yaml"
  -f "$root/docker-compose.staging.yaml"
)

workdir="$(mktemp -d)"
mkdir -p "${workdir}/tellomi"
trap 'rm -rf "$workdir"' EXIT

echo "raw compose from a directory named tellomi, no COMPOSE_PROJECT_NAME"
(
  cd "${workdir}/tellomi"
  env -u COMPOSE_PROJECT_NAME -u COMPOSE_FILE \
    "${compose[@]}" --project-directory "${workdir}/tellomi" "${files[@]}" config --format json
) > "${workdir}/from-tellomi-dir.json"

python3 - <<PY
import json
doc = json.load(open("${workdir}/from-tellomi-dir.json"))
assert doc["name"] == "tellomi-staging", doc["name"]
names = [(spec or {}).get("name") for spec in (doc.get("volumes") or {}).values()]
assert "tellomi_kafka_data" not in names, names
src = doc["services"]["kafka"]["volumes"][0]["source"]
assert src.endswith("/data-staging/kafka"), src
assert "asr" not in doc["services"]
print("directory basename tellomi did not select project tellomi")
PY

echo "COMPOSE_PROJECT_NAME=tellomi still overrides the file name"
(
  cd "${workdir}/tellomi"
  env COMPOSE_PROJECT_NAME=tellomi \
    "${compose[@]}" --project-directory "${workdir}/tellomi" "${files[@]}" config --format json
) > "${workdir}/forced-tellomi.json"
python3 - <<PY
import json
doc = json.load(open("${workdir}/forced-tellomi.json"))
assert doc["name"] == "tellomi", doc["name"]
print("confirmed env override; the wrapper must refuse it")
PY

echo "wrapper refuses the production project name"
set +e
env COMPOSE_PROJECT_NAME=tellomi "$root/scripts/staging-compose.sh" check
status=$?
set -e
if [[ "$status" -eq 0 ]]; then
  echo "wrapper accepted COMPOSE_PROJECT_NAME=tellomi" >&2
  exit 1
fi

echo "wrapper refuses COMPOSE_FILE"
set +e
env COMPOSE_FILE="$root/docker-compose.yaml" "$root/scripts/staging-compose.sh" check
status=$?
set -e
if [[ "$status" -eq 0 ]]; then
  echo "wrapper accepted COMPOSE_FILE" >&2
  exit 1
fi

echo "wrapper check renders the isolated project"
env -u COMPOSE_PROJECT_NAME -u COMPOSE_FILE -u STAGING_ENV_FILE \
  "$root/scripts/staging-compose.sh" check

echo "up without an env file is refused before any daemon call"
set +e
env -u STAGING_ENV_FILE "$root/scripts/staging-compose.sh" up -d
status=$?
set -e
if [[ "$status" -eq 0 ]]; then
  echo "up ran without STAGING_ENV_FILE" >&2
  exit 1
fi

echo "extra RSA key in the env file fails check"
extra="$(mktemp)"
printf 'TELLOMI_MTPROTO_EXTRA_KEY_FILE=/opt/tellomi/secrets/server.key\nTELLOMI_MTPROTO_EXTRA_KEY_FINGERPRINT=4381149949997195609\n' > "$extra"
set +e
out="$(env -u COMPOSE_PROJECT_NAME STAGING_ENV_FILE="$extra" "$root/scripts/staging-compose.sh" check 2>&1)"
status=$?
set -e
rm -f "$extra"
if [[ "$status" -eq 0 ]]; then
  echo "check accepted an extra RSA key" >&2
  exit 1
fi
printf '%s\n' "$out" | grep -q "TELLOMI_MTPROTO_EXTRA_KEY_FILE" || {
  echo "expected extra-key refusal, got: $out" >&2
  exit 1
}

echo "up without the tunnel DC option is refused"
dcfile="$(mktemp)"
printf 'TELLOMI_DC_OPTIONS=\n' > "$dcfile"
set +e
out="$(env -u COMPOSE_PROJECT_NAME STAGING_ENV_FILE="$dcfile" "$root/scripts/staging-compose.sh" up -d 2>&1)"
status=$?
set -e
rm -f "$dcfile"
if [[ "$status" -eq 0 ]]; then
  echo "up accepted empty TELLOMI_DC_OPTIONS" >&2
  exit 1
fi
printf '%s\n' "$out" | grep -q "refusing before any docker command" || {
  echo "expected DC refusal before docker, got: $out" >&2
  exit 1
}

echo "exact tunnel spec is accepted by the parser"
TELLOMI_DC_OPTIONS='1:127.0.0.1:10444' python3 "$root/scripts/staging_dc_options.py"

echo "parser rejects mixed, duplicate, empty, IPv6, and whitespace forms"
while IFS= read -r spec; do
  [[ -n "$spec" ]] || continue
  set +e
  out="$(TELLOMI_DC_OPTIONS="$spec" python3 "$root/scripts/staging_dc_options.py" 2>&1)"
  status=$?
  set -e
  if [[ "$status" -eq 0 ]]; then
    echo "accepted DC spec: $spec" >&2
    exit 1
  fi
  printf '%s\n' "$out" | grep -q "refusing before any docker command" || {
    echo "spec $spec did not refuse before docker: $out" >&2
    exit 1
  }
done <<'EOF'
1:8.137.39.82:10443,1:127.0.0.1:10444
1:127.0.0.1:10444,1:8.137.39.82:10443
1:127.0.0.1:10444,1:8.137.39.82:5222
1:127.0.0.1:10444,1:127.0.0.1:10444
1:127.0.0.1:10444,
,1:127.0.0.1:10444
1:::1:10444
1:2001:db8::1:10444
 1:127.0.0.1:10444
1:127.0.0.1:10444 
1: 127.0.0.1:10444
1:127.0.0.1:10443
1:127.0.0.1:11444
1:localhost:10444
1:0.0.0.0:10444
2:127.0.0.1:10444
EOF

echo "rendered mixed production DC is refused before docker"
dcfile="$(mktemp)"
printf '%s\n' 'TELLOMI_DC_OPTIONS=1:8.137.39.82:10443,1:127.0.0.1:10444' > "$dcfile"
set +e
out="$(env -u COMPOSE_PROJECT_NAME STAGING_ENV_FILE="$dcfile" "$root/scripts/staging-compose.sh" up -d 2>&1)"
status=$?
set -e
rm -f "$dcfile"
if [[ "$status" -eq 0 ]]; then
  echo "up accepted a production DC mixed with the tunnel" >&2
  exit 1
fi
printf '%s\n' "$out" | grep -q "refusing before any docker command" || {
  echo "mixed DC was not refused before docker: $out" >&2
  exit 1
}
printf '%s\n' "$out" | grep -q "8.137.39.82" || {
  echo "mixed DC was not parsed: $out" >&2
  exit 1
}
if printf '%s\n' "$out" | grep -Eq 'Cannot connect to the Docker daemon|docker.sock|Pulling|pull access denied'; then
  echo "mixed DC reached docker: $out" >&2
  exit 1
fi

echo "wrapper ignores the current directory name"
(
  cd "${workdir}/tellomi"
  env -u COMPOSE_PROJECT_NAME -u COMPOSE_FILE -u STAGING_ENV_FILE \
    "$root/scripts/staging-compose.sh" check
)

echo "staging compose config regression passed"
