#!/usr/bin/env bash
# Compile AddContact.swift for the booted simulator and insert both peers.
set -euo pipefail

root="$(cd "$(dirname "$0")/../../.." && pwd)"
sdk="$(xcrun --sdk iphonesimulator --show-sdk-path)"
arch="$(uname -m)"
out="${RUNNER_TEMP:-/tmp}/TellomiAddContact"
swiftc -sdk "$sdk" -target "${arch}-apple-ios17.0-simulator" \
  -framework Contacts \
  -o "$out" \
  "${root}/e2e/ui/ios/AddContact.swift"
xcrun simctl spawn booted "$out" "${NAME_A}" "+${PHONE_A}"
xcrun simctl spawn booted "$out" "${NAME_B}" "+${PHONE_B}"
