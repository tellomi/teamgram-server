#!/usr/bin/env bash
# Every staging Compose lifecycle command goes through here.
# A directory named tellomi, or COMPOSE_PROJECT_NAME=tellomi, must not
# select the production project or the volume tellomi_kafka_data.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
project="tellomi-staging"

if [[ "${COMPOSE_PROJECT_NAME:-}" != "" && "${COMPOSE_PROJECT_NAME}" != "$project" ]]; then
  echo "refusing COMPOSE_PROJECT_NAME=${COMPOSE_PROJECT_NAME}; it would override the staging project" >&2
  exit 1
fi
if [[ "${COMPOSE_FILE:-}" != "" ]]; then
  echo "refusing COMPOSE_FILE=${COMPOSE_FILE}; staging must use the three files this script passes" >&2
  exit 1
fi
unset COMPOSE_PROJECT_NAME COMPOSE_FILE

if docker compose version >/dev/null 2>&1; then
  compose_bin=(docker compose)
elif command -v docker-compose >/dev/null 2>&1; then
  compose_bin=(docker-compose)
else
  echo "docker compose is required for staging config checks" >&2
  exit 1
fi
compose=(
  "${compose_bin[@]}"
  -p "$project"
  --project-directory "$root"
  -f "$root/docker-compose-env.yaml"
  -f "$root/docker-compose.yaml"
  -f "$root/docker-compose.staging.yaml"
)
if [[ -n "${STAGING_ENV_FILE:-}" ]]; then
  compose+=(--env-file "$STAGING_ENV_FILE")
fi

usage() {
  echo "usage: scripts/staging-compose.sh check|config|up|down|ps|logs|pull ..." >&2
  exit 2
}

[[ $# -ge 1 ]] || usage
cmd="$1"
shift

case "$cmd" in
  check|config|up|down|ps|logs|pull|stop|start|rm) ;;
  *)
    echo "refusing staging command: $cmd" >&2
    exit 2
    ;;
esac

render() {
  "${compose[@]}" config --format json
}

assert_isolated() {
  RENDERED="$1" python3 - <<'PY'
import json, os, sys
doc = json.loads(os.environ["RENDERED"])
name = doc.get("name")
if name != "tellomi-staging":
    sys.exit(f"project name {name!r}, want tellomi-staging")
services = doc.get("services") or {}
volumes = doc.get("volumes") or {}
for key, spec in volumes.items():
    actual = (spec or {}).get("name") or key
    if actual == "tellomi_kafka_data" or actual.startswith("tellomi_"):
        sys.exit(f"volume {actual} collides with the production project")
kafka_binds = []
for vol in (services.get("kafka") or {}).get("volumes") or []:
    if isinstance(vol, dict) and str(vol.get("target")) == "/var/lib/kafka/data":
        kafka_binds.append(str(vol.get("source") or ""))
if not any(src.endswith("/data-staging/kafka") or src.endswith("\\data-staging\\kafka") for src in kafka_binds):
    sys.exit(f"kafka data is not under data-staging: {kafka_binds}")
for spec in services.values():
    for vol in spec.get("volumes") or []:
        if not isinstance(vol, dict):
            continue
        src = str(vol.get("source") or "")
        if "/opt/tellomi/teamgram-server/data/" in src:
            sys.exit(f"production data path in staging config: {src}")
want = {"etcd", "kafka", "mysql", "mysql-migrate", "redis", "teamgram"}
got = set(services)
if got != want:
    sys.exit(f"default services {sorted(got)}, want {sorted(want)}")
for forbidden in ("asr", "coturn", "mediamtx", "minio", "jaeger", "prometheus",
                  "grafana", "elasticsearch", "kibana", "filebeat", "go-stash",
                  "node-exporter"):
    if forbidden in got:
        sys.exit(f"{forbidden} must not start in the desktop staging project")
teamgram = services["teamgram"]
ports = teamgram.get("ports") or []
published = []
for item in ports:
    if isinstance(item, dict):
        published.append((item.get("host_ip"), str(item.get("published")), str(item.get("target"))))
    else:
        published.append(str(item))
need = {("127.0.0.1", "10444", "10443"), ("127.0.0.1", "11444", "11443")}
if set(published) != need:
    sys.exit(f"teamgram ports {published}, want {sorted(need)}")
for service, spec in services.items():
    if service == "teamgram":
        continue
    if spec.get("ports"):
        sys.exit(f"{service} publishes host ports: {spec.get('ports')}")
for service in ("teamgram", "mysql", "kafka", "redis", "etcd"):
    options = ((services[service].get("logging") or {}).get("options") or {})
    if not options.get("max-size"):
        sys.exit(f"{service} has no json-file max-size")
caps = {
    "teamgram": 1024 * 1024 * 1024,
    "mysql": 512 * 1024 * 1024,
    "kafka": 768 * 1024 * 1024,
    "redis": 256 * 1024 * 1024,
    "etcd": 128 * 1024 * 1024,
}
def as_bytes(value):
    if isinstance(value, bool) or value is None:
        return None
    if isinstance(value, int):
        return value
    if isinstance(value, float):
        return int(value)
    text = str(value).strip().lower()
    if text.endswith("mb"):
        return int(float(text[:-2]) * 1024 * 1024)
    if text.endswith("m"):
        return int(float(text[:-1]) * 1024 * 1024)
    if text.endswith("g"):
        return int(float(text[:-1]) * 1024 * 1024 * 1024)
    try:
        return int(text)
    except ValueError:
        return None

for service, limit in caps.items():
    mem = ((services[service].get("deploy") or {}).get("resources") or {}).get("limits") or {}
    raw = as_bytes(services[service].get("mem_limit"))
    if raw != limit and as_bytes(mem.get("memory")) != limit:
        sys.exit(f"{service} memory {services[service].get('mem_limit')!r} / {mem!r}, want {limit}")
cpus = ((teamgram.get("deploy") or {}).get("resources") or {}).get("limits", {}).get("cpus")
nano = teamgram.get("cpus")
if nano not in (1, 1.0, "1", 1000000000) and cpus not in (1, 1.0, "1"):
    sys.exit(f"teamgram cpus {nano!r} / {cpus!r}, want 1")

def env_get(spec, key):
    env = spec.get("environment") or {}
    if isinstance(env, dict):
        return str(env.get(key) or "")
    if isinstance(env, list):
        prefix = key + "="
        for item in env:
            if isinstance(item, str) and item.startswith(prefix):
                return item[len(prefix):]
    return ""

for key in ("TELLOMI_MTPROTO_EXTRA_KEY_FILE", "TELLOMI_MTPROTO_EXTRA_KEY_FINGERPRINT"):
    if env_get(teamgram, key).strip():
        sys.exit(f"{key} is set; staging must not load an extra RSA key")
for spec in services.values():
    for vol in spec.get("volumes") or []:
        if isinstance(vol, dict) and "/opt/tellomi/secrets" in str(vol.get("source") or ""):
            sys.exit("staging config mounts /opt/tellomi/secrets")
sms = env_get(teamgram, "TELLOMI_SMS_PROVIDER").strip().lower()
if sms == "aliyun":
    sys.exit("TELLOMI_SMS_PROVIDER=aliyun is refused for staging")
if env_get(teamgram, "TELLOMI_RESEND_API_KEY").strip():
    sys.exit("TELLOMI_RESEND_API_KEY is refused for staging")
print("staging compose isolated: project tellomi-staging", file=sys.stderr)
PY
}

assert_up_dc() {
  RENDERED="$1" python3 - <<'PY'
import json, os, sys
doc = json.loads(os.environ["RENDERED"])
teamgram = (doc.get("services") or {})["teamgram"]
env = teamgram.get("environment") or {}
def env_get(key):
    if isinstance(env, dict):
        return str(env.get(key) or "")
    prefix = key + "="
    if isinstance(env, list):
        for item in env:
            if isinstance(item, str) and item.startswith(prefix):
                return item[len(prefix):]
    return ""
dc = env_get("TELLOMI_DC_OPTIONS")
if "1:127.0.0.1:10444" not in dc:
    sys.exit(
        "TELLOMI_DC_OPTIONS must include 1:127.0.0.1:10444 "
        "(desktop SSH tunnel). Unset leaves help.getConfig dc_options empty."
    )
PY
}

rendered="$(render)"
assert_isolated "$rendered"

if [[ "$cmd" == "up" ]]; then
  if [[ -z "${STAGING_ENV_FILE:-}" ]]; then
    echo "up requires STAGING_ENV_FILE so DC options and secrets are explicit" >&2
    exit 1
  fi
  assert_up_dc "$rendered"
fi

if [[ "$cmd" == "check" ]]; then
  exit 0
fi
if [[ "$cmd" == "config" ]]; then
  printf '%s\n' "$rendered"
  exit 0
fi
exec "${compose[@]}" "$cmd" "$@"
