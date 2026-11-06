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
import json, os, subprocess, sys, tempfile

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
    "teamgram": 3584 * 1024 * 1024,
    "mysql": 1536 * 1024 * 1024,
    "kafka": 1717986918,  # 1.6g as Compose renders it (1.6 * 1024^3)
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
    swap = int(full["services"][name].get("memswap_limit") or 0)
    if swap != want:
        raise SystemExit(f"{name} memswap_limit {swap} != {want}")
    deploy = full["services"][name]["deploy"]["resources"]["limits"]["memory"]
    if int(deploy) != want:
        raise SystemExit(f"{name} deploy memory {deploy} != {want}")

mysql_cmd = full["services"]["mysql"].get("command") or ""
if isinstance(mysql_cmd, list):
    mysql_cmd = " ".join(mysql_cmd)
for flag in ("utf8mb4", "mysql_native_password", "innodb-buffer-pool-size=1G"):
    if flag not in mysql_cmd:
        raise SystemExit(f"mysql command missing {flag}")

bare_heap = bare["services"]["kafka"]["environment"].get("KAFKA_HEAP_OPTS", "")
if bare_heap:
    raise SystemExit(f"base compose sets KAFKA_HEAP_OPTS {bare_heap!r}")
heap = full["services"]["kafka"]["environment"].get("KAFKA_HEAP_OPTS", "")
if heap != "-Xmx1G -Xms1G":
    raise SystemExit(f"KAFKA_HEAP_OPTS is {heap!r}")
roles = full["services"]["kafka"]["environment"].get("KAFKA_PROCESS_ROLES", "")
if roles != "broker,controller":
    raise SystemExit(f"kafka environment was replaced; KAFKA_PROCESS_ROLES is {roles!r}")

def entrypoint_text(doc):
    ep = doc["services"]["coturn"].get("entrypoint")
    if isinstance(ep, list):
        return "\n".join(str(part) for part in ep)
    return ep or ""

def mounts_generator(doc):
    for vol in doc["services"]["coturn"].get("volumes") or []:
        target = vol.get("target") if isinstance(vol, dict) else ""
        if target == "/opt/coturn/entrypoint.sh":
            return True
    return False

# `docker compose config` re-escapes `$` as `$$`, so the runtime `$@`
# shows up here as `$$@`.
text = entrypoint_text(bare)
if "[ -f /opt/coturn/entrypoint.sh ]" not in text:
    raise SystemExit("coturn entrypoint does not test for the mounted script")
if 'exec docker-entrypoint.sh "$$@"' not in text:
    raise SystemExit("coturn entrypoint has no image fallback")
ep = bare["services"]["coturn"]["entrypoint"]
if not isinstance(ep, list) or ep[0] != "/bin/sh" or ep[-1] != "docker-entrypoint.sh":
    raise SystemExit(f"coturn entrypoint lost the $0 placeholder: {ep!r}")
if not mounts_generator(bare):
    raise SystemExit("default compose does not mount the coturn generator")
if entrypoint_text(full) != text:
    raise SystemExit("limits overlay changed the coturn entrypoint")

override = tempfile.NamedTemporaryFile("w", suffix=".yaml", delete=False)
override.write("services:\n  coturn:\n    volumes: !override []\n")
override.close()
try:
    dropped = render([
        "-f", "docker-compose-env.yaml",
        "-f", "docker-compose.yaml",
        "-f", override.name,
    ])
finally:
    os.unlink(override.name)
if dropped["services"]["coturn"].get("volumes"):
    raise SystemExit("volumes: !override did not drop the coturn script mount")
dropped_text = entrypoint_text(dropped)
if 'exec docker-entrypoint.sh "$$@"' not in dropped_text:
    raise SystemExit("coturn fallback missing after volumes are replaced")

print("check-compose-limits: ok")
PY
