#!/usr/bin/env bash
# Post-deploy smoke test. Talks to a running server as the official clients do
# and runs the subset that does not restart the process.
#
#   TELLOMI_E2E_ADDR=127.0.0.1:10443 ./scripts/smoke-e2e.sh
#   TELLOMI_E2E_ADDR=127.0.0.1:10444 ./scripts/smoke-e2e.sh   # staging
#
# The server must accept the default 99966 test-phone range (docs/e2e.md).
# TELLOMI_E2E_ADDR, TELLOMI_E2E_PHONE_*, and TELLOMI_E2E_CODE_* override
# e2e/config.json. This script does not set TELLOMI_E2E_RESTART.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
export TELLOMI_E2E=1
export TELLOMI_E2E_ROOT="$root"
unset TELLOMI_E2E_RESTART

cd "$root/e2e"
exec go test -count=1 -timeout 20m -v \
  -run 'TestInitConnectionInvokeWithLayer|TestHelpGetConfig|TestHelpGetAppConfig|TestHelpGetTermsOfServiceUpdate|TestUpdatesGetStateAndDifference|TestStartupRPCs|TestPrivateChatReadOutbox|TestSendReactionThumbsUp|TestLinksUseTellCC|TestAvailableReactionsDownloadable|TestStickerSetUploadGetFile|TestUsernameThreeCharacters' \
  .
