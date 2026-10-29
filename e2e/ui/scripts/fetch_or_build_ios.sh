#!/usr/bin/env bash
# Print the path of a Tellomi iOS Simulator .app. Logs go to stderr.
# Prefer IOS_SIMULATOR_APP_URL, then a simulator zip on the latest
# tellomi/Telegram-iOS release, then a local debug_sim_arm64 build.
# The build edits Tellomi/tellomi-server.json only in the CI checkout.
set -euo pipefail

root="$(cd "$(dirname "$0")/../../.." && pwd)"
host="${E2E_API_HOST:-staging-api.tellomi.cn}"
work="${RUNNER_TEMP:-/tmp}/tellomi-ios-fetch"
mkdir -p "$work"

download_and_select() {
  local url="$1"
  local zip="${work}/simulator-app.zip"
  echo "downloading simulator app" >&2
  curl -fsSL --retry 3 -o "$zip" "$url"
  rm -rf "${work}/unzipped"
  mkdir -p "${work}/unzipped"
  unzip -q "$zip" -d "${work}/unzipped"
  python3 "${root}/e2e/ui/scripts/select_simulator_app.py" "${work}/unzipped"
}

if [[ -n "${IOS_SIMULATOR_APP_URL:-}" ]]; then
  download_and_select "${IOS_SIMULATOR_APP_URL}"
  exit 0
fi

python3 - <<'PY' > "${work}/simulator-url.txt" || true
import json, os, urllib.request
request = urllib.request.Request(
    "https://api.github.com/repos/tellomi/Telegram-iOS/releases/latest",
    headers={"Accept": "application/vnd.github+json", "User-Agent": "tellomi-e2e"},
)
token = os.environ.get("TELLOMI_GH_TOKEN", "").strip()
if token:
    request.add_header("Authorization", "Bearer " + token)
try:
    with urllib.request.urlopen(request, timeout=60) as response:
        payload = json.load(response)
except Exception:
    raise SystemExit(0)
for asset in payload.get("assets", []):
    name = str(asset.get("name", "")).lower()
    if name.endswith(".zip") and ("sim" in name or "simulator" in name):
        print(asset.get("browser_download_url", ""))
        break
PY
sim_url="$(tr -d '[:space:]' < "${work}/simulator-url.txt" || true)"
if [[ -n "$sim_url" ]]; then
  download_and_select "$sim_url"
  exit 0
fi

src="${IOS_SRC:-${RUNNER_TEMP:-/tmp}/Telegram-iOS}"
{
  ref="${IOS_GIT_REF:-master}"
  echo "building tellomi/Telegram-iOS ${ref} (${host}) at ${src}"
  rm -rf "$src"
  git clone --depth 1 --branch "$ref" https://github.com/tellomi/Telegram-iOS.git "$src"
  cd "$src"
  if [[ -f .github/checkout-submodules.sh ]]; then
    bash .github/checkout-submodules.sh
  else
    git submodule update --init --recursive
  fi
  python3 "${root}/e2e/ui/scripts/seeds.py" "${src}/Tellomi/tellomi-server.json" "$host"
  python3 Tellomi/generate_server_config.py
  python3 Tellomi/check_server_config.py
  xcode_version="$(python3 -c 'import json; print(json.load(open("versions.json"))["xcode"])')"
  candidate="/Applications/Xcode_${xcode_version}.app"
  if [[ ! -d "$candidate" ]]; then
    echo "Xcode ${xcode_version} is not installed. Set repository variable E2E_IOS_RUNNER to the xcode-27 runner used by tellomi/Telegram-iOS TestFlight."
    ls /Applications | grep Xcode || true
    exit 1
  fi
  sudo xcode-select -s "${candidate}/Contents/Developer"
  xcodebuild -version
  df -h /
  cache="${BAZEL_CACHE:-${RUNNER_TEMP:-/tmp}/telegram-bazel-cache}"
  mkdir -p "$cache"
  python3 -u build-system/Make/Make.py \
    --overrideXcodeVersion \
    --cacheDir "$cache" \
    build \
    --configurationPath build-system/appstore-configuration.json \
    --disableProvisioningProfiles \
    --buildNumber=1 \
    --configuration=debug_sim_arm64
} >&2
python3 "${root}/e2e/ui/scripts/select_simulator_app.py" "$src"
