#!/usr/bin/env bash
# Build JobShoutMac and run package tests. Mac XCUITests need a real Apple
# Development team in Xcode (unsigned runners are rejected by Gatekeeper as
# "damaged") — run those via Product → Test after setting the team.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "==> xcodegen"
command -v xcodegen >/dev/null || brew install xcodegen
xcodegen generate

SIGN=(
  CODE_SIGN_IDENTITY=-
  CODE_SIGN_STYLE=Manual
  DEVELOPMENT_TEAM=
  CODE_SIGNING_ALLOWED=NO
)
DD="${JOBSHOUT_MAC_DERIVED_DATA:-/tmp/JobShoutMacTestDD}"

echo "==> JobShoutKit swift test"
(cd JobShoutKit && swift test)

echo "==> Build JobShoutMac"
xcodebuild build \
  -project JobShout.xcodeproj -scheme JobShoutMac -configuration Debug \
  -destination 'generic/platform=macOS' \
  -derivedDataPath "$DD" \
  -skipPackagePluginValidation \
  "${SIGN[@]}"

APP="$DD/Build/Products/Debug/JobShout.app"
test -d "$APP"
echo "==> Built $APP"
echo "==> JobShout Mac build OK (UI tests: open JobShoutMac scheme in Xcode with your team → Product → Test)"
