#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$root"
artifact_dir="${ARTIFACT_DIR:-${root}/ui-artifacts/ios}"
mkdir -p "$artifact_dir"
export E2E_API_HOST="${E2E_API_HOST:-staging-api.tellomi.cn}"
export E2E_API_PORT="${E2E_API_PORT:-10443}"
export PATH="${PATH}:${HOME}/.maestro/bin"

python3 e2e/ui/scripts/redirect.py --preflight
staging_ip="$(python3 -c 'import os, socket; print(socket.gethostbyname(os.environ.get("E2E_API_HOST", "staging-api.tellomi.cn")))')"
# Prebuilt clients dial the hostname api.tellomi.cn. Send that name to staging.
# A from-source build dials E2E_API_HOST directly.
sudo sed -i '' '/[[:space:]]api\.tellomi\.cn$/d' /etc/hosts
echo "${staging_ip} api.tellomi.cn" | sudo tee -a /etc/hosts >/dev/null
sudo dscacheutil -flushcache || true
sudo killall -HUP mDNSResponder || true

python3 e2e/ui/scripts/accounts.py --write "${artifact_dir}/accounts.env"
set -a
# shellcheck disable=SC1090
source "${artifact_dir}/accounts.env"
set +a

app="$(bash e2e/ui/scripts/fetch_or_build_ios.sh)"
echo "simulator app: ${app}"

udid="$(python3 - <<'PY'
import json, subprocess, sys
raw = subprocess.check_output(["xcrun", "simctl", "list", "devices", "available", "-j"], text=True)
chosen = []
for runtime, devices in json.loads(raw).get("devices", {}).items():
    if "iOS" not in runtime:
        continue
    for device in devices:
        name = device.get("name") or ""
        if device.get("isAvailable", True) and name.startswith("iPhone") and "SE" not in name:
            chosen.append(name + "\t" + device["udid"])
if not chosen:
    sys.exit("no available iPhone simulator")
chosen.sort()
print(chosen[-1].split("\t", 1)[1])
PY
)"
xcrun simctl boot "$udid" || true
xcrun simctl bootstatus "$udid" -b
xcrun simctl spawn booted defaults write NSGlobalDomain AppleLanguages -array zh-Hans
xcrun simctl spawn booted defaults write NSGlobalDomain AppleLocale -string zh_CN
xcrun simctl shutdown booted || true
xcrun simctl boot "$udid"
xcrun simctl bootstatus "$udid" -b
xcrun simctl install booted "$app"
xcrun simctl privacy booted grant contacts app.tellomi.chat || true
xcrun simctl privacy booted grant photos app.tellomi.chat || true
xcrun simctl addmedia booted "${root}/e2e/ui/fixtures/group-avatar.png" || true

if ! bash e2e/ui/scripts/ios_add_swift_contact.sh; then
  python3 e2e/ui/scripts/ios_contacts.py
fi

bash e2e/ui/scripts/install_maestro.sh
set +e
bash e2e/ui/scripts/run_maestro.sh ios "$artifact_dir"
status=$?
set -e
xcrun simctl io booted screenshot "${artifact_dir}/final.png" || true
find "$artifact_dir" -name '*.png' | grep -q . || {
  echo "no screenshots were written" >&2
  exit 1
}
exit "$status"
