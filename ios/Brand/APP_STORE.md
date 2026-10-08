# Mac App Store readiness (JobShout.app)

## Product

| | |
|---|---|
| Display name | JobShout |
| Product / `.app` name | `JobShout.app` (`PRODUCT_NAME`) |
| Scheme | `JobShoutMac` |
| Bundle IDs | `com.jobshout.mac.dev` / `.staging` / `com.jobshout.mac` |
| Category (suggested) | Productivity / Business |
| Min macOS | 15.0 |

## Brand assets

Masters are built from [`docs/brand/`](../../docs/brand/BRAND.md) by `docs/brand/tools/build-brand.py`. iPhone, iPad and Watch use the full-bleed `AppIcon-1024.png`; the Mac uses `AppIcon-mac-1024.png`, which has the macOS rounded-square body and shadow baked in, plus a simplified small icon for 16–32 px.

## Local signed `.app` (not App Store yet)

Open in Xcode, set your **Development Team** on JobShoutMac, then:

```sh
cd ios && xcodegen generate
xcodebuild -project JobShout.xcodeproj -scheme JobShoutMac -configuration Release \
  -destination 'generic/platform=macOS' \
  -archivePath /tmp/JobShout.xcarchive archive
xcodebuild -exportArchive -archivePath /tmp/JobShout.xcarchive \
  -exportPath /tmp/JobShoutExport \
  -exportOptionsPlist Brand/ExportOptions-MacAppStore.plist
```

Until App Store Connect + distribution certs exist, Archive → Distribute App → **Development** or **Direct Distribution**, or Product → Run with Automatic signing.

## Checklist before submit

- [ ] App icons: 1024 master in catalog (done); verify Dock in a **signed** Release build
- [ ] Accent / brand colours match `BRAND.md` (done)
- [ ] Sandbox + network client entitlements (already in `JobShoutMac.entitlements`)
- [ ] Sign in with Apple capability on the Mac App ID (optional if Google-only on Mac)
- [ ] Privacy Nutrition Labels / `PrivacyInfo.xcprivacy` if required by APIs used
- [ ] App Store screenshots (1280×800 / 2560×1600 / 2880×1800)
- [ ] Review notes: int vs prod ring; Google OAuth redirect `jobshout://`
- [ ] Production `APPLE_CLIENT_IDS` includes `com.jobshout.mac`
- [ ] Notarization / App Store Connect record for `com.jobshout.mac`

## ExportOptions (App Store)

See `ExportOptions-MacAppStore.plist` — fill `teamID` before export.
