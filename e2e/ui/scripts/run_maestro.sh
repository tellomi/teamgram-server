#!/usr/bin/env bash
# Run the checklist. Later flows still run after an earlier failure so the
# screenshot artifact covers every step that could start.
set -u

platform="${1:?platform}"
artifact_dir="${2:?artifact dir}"
root="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$root"
export PATH="${PATH}:${HOME}/.maestro/bin"
export MAESTRO_CLI_NO_ANALYTICS=1
export MAESTRO_CLI_ANALYSIS_NOTIFICATION_DISABLED=true
mkdir -p "$artifact_dir"

args=(-e "PLATFORM=${platform}")
while IFS= read -r line; do
  [[ "$line" =~ ^[A-Z0-9_]+= ]] || continue
  args+=(-e "$line")
done < "${artifact_dir}/accounts.env"

status=0
run() {
  maestro test --debug-output "$artifact_dir" "${args[@]}" "$1" || status=1
}

run e2e/ui/maestro/signup-agreement.yaml
run e2e/ui/maestro/messages-before-read.yaml

dump="${artifact_dir}/hierarchy.json"
if ! maestro hierarchy >"$dump" 2>"${artifact_dir}/hierarchy.err"; then
  if [[ "$platform" == "android" ]]; then
    adb shell uiautomator dump /sdcard/uidump.xml || true
    adb pull /sdcard/uidump.xml "${artifact_dir}/hierarchy.xml" || true
    if [[ -s "${artifact_dir}/hierarchy.xml" ]]; then
      dump="${artifact_dir}/hierarchy.xml"
    fi
  fi
fi
if [[ -s "$dump" ]]; then
  python3 e2e/ui/scripts/hierarchy.py assert "$dump" || status=1
else
  echo "no UI hierarchy was captured; message order was not checked" >&2
  status=1
fi

run e2e/ui/maestro/after-read.yaml

if [[ -d "${HOME}/.maestro/tests" ]]; then
  mkdir -p "${artifact_dir}/maestro-debug"
  cp -R "${HOME}/.maestro/tests/." "${artifact_dir}/maestro-debug/" || true
fi

exit "$status"
