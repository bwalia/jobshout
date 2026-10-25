#!/usr/bin/env bash
# Archive JobShout (iOS Release), export an App Store IPA, and upload to
# App Store Connect when credentials are available.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

TEAM="${DEVELOPMENT_TEAM:-PAS2QUVJHC}"
ARCHIVE_PATH="${ARCHIVE_PATH:-/tmp/JobShout-iOS.xcarchive}"
EXPORT_PATH="${EXPORT_PATH:-/tmp/JobShout-iOS-Export}"
EXPORT_OPTIONS="${EXPORT_OPTIONS:-$ROOT/Brand/ExportOptions-iOSAppStore.plist}"
SCHEME="${SCHEME:-JobShout}"
CONFIGURATION="${CONFIGURATION:-Release}"
# Set JOBSHOUT_UPLOAD=1 to push to App Store Connect (requires ASC app record).
UPLOAD="${JOBSHOUT_UPLOAD:-0}"

# Homebrew rsync 3.x breaks xcodebuild -exportArchive ("Copy failed"). Prefer
# macOS /usr/bin/rsync (openrsync) for packaging and upload.
export PATH="/usr/bin:/bin:/usr/sbin:/sbin:${PATH}"

echo "==> xcodegen"
command -v xcodegen >/dev/null || brew install xcodegen
xcodegen generate

echo "==> Archive $SCHEME ($CONFIGURATION) team=$TEAM"
rm -rf "$ARCHIVE_PATH" "$EXPORT_PATH"
xcodebuild archive \
  -project JobShout.xcodeproj \
  -scheme "$SCHEME" \
  -configuration "$CONFIGURATION" \
  -destination 'generic/platform=iOS' \
  -archivePath "$ARCHIVE_PATH" \
  -skipPackagePluginValidation \
  -allowProvisioningUpdates \
  DEVELOPMENT_TEAM="$TEAM" \
  CODE_SIGN_STYLE=Automatic

echo "==> Export IPA"
xcodebuild -exportArchive \
  -archivePath "$ARCHIVE_PATH" \
  -exportPath "$EXPORT_PATH" \
  -exportOptionsPlist "$EXPORT_OPTIONS" \
  -allowProvisioningUpdates

IPA="$(find "$EXPORT_PATH" -maxdepth 1 -name '*.ipa' | head -1 || true)"
if [[ -z "${IPA:-}" ]]; then
  echo "error: no .ipa in $EXPORT_PATH" >&2
  ls -la "$EXPORT_PATH" >&2 || true
  exit 1
fi
echo "==> IPA: $IPA"

upload_with_api_key() {
  local key_id="${APP_STORE_CONNECT_KEY_ID:-${ASC_KEY_ID:-}}"
  local issuer_id="${APP_STORE_CONNECT_ISSUER_ID:-${ASC_ISSUER_ID:-}}"
  local key_path="${APP_STORE_CONNECT_KEY_PATH:-${ASC_KEY_PATH:-}}"
  if [[ -z "$key_id" || -z "$issuer_id" || -z "$key_path" ]]; then
    return 1
  fi
  if [[ ! -f "$key_path" ]]; then
    echo "error: ASC key file not found: $key_path" >&2
    return 1
  fi
  echo "==> Upload via altool (API key)"
  xcrun altool --upload-app \
    --type ios \
    --file "$IPA" \
    --apiKey "$key_id" \
    --apiIssuer "$issuer_id"
}

upload_via_xcode() {
  local upload_opts
  upload_opts="$(mktemp /tmp/JobShout-UploadOptions.XXXXXX.plist)"
  cat >"$upload_opts" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>method</key>
	<string>app-store-connect</string>
	<key>destination</key>
	<string>upload</string>
	<key>signingStyle</key>
	<string>automatic</string>
	<key>teamID</key>
	<string>${TEAM}</string>
	<key>uploadSymbols</key>
	<true/>
</dict>
</plist>
EOF
  echo "==> Upload via xcodebuild (destination=upload)"
  local upload_path="${EXPORT_PATH}-upload"
  rm -rf "$upload_path"
  xcodebuild -exportArchive \
    -archivePath "$ARCHIVE_PATH" \
    -exportPath "$upload_path" \
    -exportOptionsPlist "$upload_opts" \
    -allowProvisioningUpdates
  rm -f "$upload_opts"
}

if [[ "$UPLOAD" == "1" ]]; then
  if upload_with_api_key || upload_via_xcode; then
    echo "==> Uploaded to App Store Connect. Wait for processing, then enable in TestFlight."
    exit 0
  fi
  echo "error: upload failed. Create the App Store Connect app for com.jobshout.app first" >&2
  echo "       (see Brand/TESTFLIGHT.md), then re-run with JOBSHOUT_UPLOAD=1." >&2
  exit 1
fi

if upload_with_api_key; then
  echo "==> Uploaded to App Store Connect. Wait for processing, then enable in TestFlight."
  exit 0
fi

echo ""
echo "==> CLI upload skipped. Create the ASC app for com.jobshout.app if missing, then either:"
echo "      JOBSHOUT_UPLOAD=1 ./scripts/release-ios-testflight.sh"
echo "    or set APP_STORE_CONNECT_KEY_ID / ISSUER_ID / KEY_PATH (.p8), or:"
echo "      1. open $ARCHIVE_PATH"
echo "      2. Distribute App → App Store Connect → Upload"
echo "      3. TestFlight → enable build for internal testers"
echo ""
echo "==> Archive ready: $ARCHIVE_PATH"
echo "==> IPA ready:     $IPA"
