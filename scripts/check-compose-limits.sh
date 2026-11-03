#!/usr/bin/env bash
# Render the opt-in limits overlay and check the memory values (D-34, M7).
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

if ! command -v docker >/dev/null 2>&1 || ! docker compose version >/dev/null 2>&1; then
  echo "docker compose is required" >&2
  exit 1
fi

export COMPOSE_DISABLE_ENV_FILE=1

fail() {
  echo "check-compose-limits: $*" >&2
  exit 1
}

base=( -f docker-compose-env.yaml -f docker-compose.yaml )
with=( "${base[@]}" -f docker-compose.limits.yaml )

docker compose "${base[@]}" config --quiet
docker compose "${with[@]}" config --quiet

python3 - "${base[@]}" -- "${with[@]}" <<'PY'
import json, subprocess, sys

def render(files):
    cmd = ["docker", "compose", *files, "config", "--format", "json"]
    out = subprocess.check_output(cmd, text=True)
    return json.loads(out)

sep = sys.argv.index("--")
bare = render(sys.argv[1:sep])
full = render(sys.argv[sep + 1:])

def mem(doc, name):
    svc = doc["services"][name]
    return int(svc.get("mem_limit") or 0)

# Without the overlay, teamgram has no cap. asr already has 1.5 GiB.
if mem(bare, "teamgram") != 0:
    raise SystemExit("teamgram mem_limit is set without the overlay")
if mem(bare, "asr") != 1536 * 1024 * 1024:
    raise SystemExit("asr limit changed in the base compose")

expect = {
    "teamgram": 2560 * 1024 * 1024,
    "mysql": 1536 * 1024 * 1024,
    "kafka": 1288490188,  # 1.2g as Compose renders it
    "redis": 512 * 1024 * 1024,
    "etcd": 256 * 1024 * 1024,
    "coturn": 256 * 1024 * 1024,
    "mediamtx": 256 * 1024 * 1024,
    "asr": 1536 * 1024 * 1024,
}
for name, want in expect.items():
    got = mem(full, name)
    if got != want:
        raise SystemExit(f"{name} mem_limit {got} != {want}")
    deploy = full["services"][name]["deploy"]["resources"]["limits"]["memory"]
    if int(deploy) != want:
        raise SystemExit(f"{name} deploy memory {deploy} != {want}")

mysql_cmd = full["services"]["mysql"].get("command") or ""
if isinstance(mysql_cmd, list):
    mysql_cmd = " ".join(mysql_cmd)
for flag in ("utf8mb4", "mysql_native_password", "innodb-buffer-pool-size=1G"):
    if flag not in mysql_cmd:
        raise SystemExit(f"mysql command missing {flag}")

heap = full["services"]["kafka"]["environment"].get("KAFKA_HEAP_OPTS", "")
if heap != "-Xmx768m -Xms768m":
    raise SystemExit(f"KAFKA_HEAP_OPTS is {heap!r}")

print("check-compose-limits: ok")
PY
