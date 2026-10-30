#!/usr/bin/env bash
# Runs inside reactivecircus/android-emulator-runner, after the emulator boots.
set -euo pipefail

root="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$root"
artifact_dir="${ARTIFACT_DIR:-${root}/ui-artifacts/android}"
mkdir -p "$artifact_dir"
export E2E_API_HOST="${E2E_API_HOST:-staging-api.tellomi.cn}"
export E2E_API_PORT="${E2E_API_PORT:-11443}"
export PATH="${PATH}:${HOME}/.maestro/bin"

bash e2e/ui/scripts/install_maestro.sh
python3 e2e/ui/scripts/accounts.py --write "${artifact_dir}/accounts.env"
set -a
# shellcheck disable=SC1090
source "${artifact_dir}/accounts.env"
set +a

adb wait-for-device
adb root || true
adb wait-for-device
python3 e2e/ui/scripts/redirect.py --apply

if [[ -z "${APK_PATH:-}" || ! -f "${APK_PATH}" ]]; then
  echo "APK_PATH is not set to a downloaded tellomi APK." >&2
  exit 1
fi
adb install -r -d "${APK_PATH}"
adb shell pm grant app.tellomi.chat android.permission.READ_CONTACTS || true
adb shell pm grant app.tellomi.chat android.permission.WRITE_CONTACTS || true
adb shell pm grant app.tellomi.chat android.permission.POST_NOTIFICATIONS || true
adb shell pm grant app.tellomi.chat android.permission.READ_MEDIA_IMAGES || true
adb shell settings put system system_locales zh-CN || true
python3 e2e/ui/scripts/android_contacts.py
adb shell mkdir -p /sdcard/Pictures
adb push e2e/ui/fixtures/group-avatar.png /sdcard/Pictures/group-avatar.png
adb shell am broadcast -a android.intent.action.MEDIA_SCANNER_SCAN_FILE -d file:///sdcard/Pictures/group-avatar.png || true

set +e
bash e2e/ui/scripts/run_maestro.sh android "$artifact_dir"
status=$?
set -e
adb exec-out screencap -p > "${artifact_dir}/final.png" || true
adb logcat -d -t 400 > "${artifact_dir}/logcat.txt" || true
find "$artifact_dir" -name '*.png' | grep -q . || {
  echo "no screenshots were written" >&2
  exit 1
}
exit "$status"
