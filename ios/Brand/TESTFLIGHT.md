# iPhone TestFlight (JobShout)

Primary TestFlight track: **Release** → prod API → bundle `com.jobshout.app`.

| | |
|---|---|
| Display name | JobShout |
| Scheme | `JobShout` |
| Configuration | Release |
| Bundle ID | `com.jobshout.app` |
| App Store Connect Apple ID | `511622488` |
| ASC URL | https://appstoreconnect.apple.com/apps/511622488 |
| Marketing version | `0.1.0` (`MARKETING_VERSION` in `project.yml`) |
| Build | `CURRENT_PROJECT_VERSION` (bump each upload) |
| Min iOS | 18.0 |
| Team | `PAS2QUVJHC` (pass via script/env; never commit in `project.yml`) |

## One-shot archive + export

```sh
cd ios && ./scripts/release-ios-testflight.sh
```

Override team or paths if needed:

```sh
DEVELOPMENT_TEAM=PAS2QUVJHC \
  ARCHIVE_PATH=/tmp/JobShout-iOS.xcarchive \
  EXPORT_PATH=/tmp/JobShout-iOS-Export \
  ./scripts/release-ios-testflight.sh
```

After the ASC app record exists:

```sh
JOBSHOUT_UPLOAD=1 ./scripts/release-ios-testflight.sh
```

Or set `APP_STORE_CONNECT_KEY_ID`, `APP_STORE_CONNECT_ISSUER_ID`, and
`APP_STORE_CONNECT_KEY_PATH` (`.p8`). If CLI upload still fails, open the
archive in **Xcode → Organizer → Distribute App → App Store Connect → Upload**.

Note: Homebrew `rsync` 3.x breaks export (`Copy failed`); the script puts
`/usr/bin` first on `PATH` so macOS openrsync is used.

## App Store Connect / Developer Portal checklist

Use existing ASC app **Apple ID `511622488`** (bundle must be `com.jobshout.app`).
Override with `ASC_APPLE_ID=…` if needed.

- [ ] App ID `com.jobshout.app` with **Sign in with Apple**, **Push Notifications**, **Keychain Sharing** (`$(AppIdentifierPrefix)com.jobshout.shared`)
- [x] App Store Connect iOS app record Apple ID `511622488`
- [ ] Xcode signed into team `PAS2QUVJHC` so Automatic signing can create **Apple Distribution** + App Store provisioning profile
- [ ] Optional: App Store Connect API key (Issuer ID + Key ID + `.p8`) for non-interactive `altool` / Transporter upload
- [ ] Prod Helm `apple.clientIds` includes `com.jobshout.app` (and Mac/Watch ids when those ship)
- [ ] Privacy: `ITSAppUsesNonExemptEncryption` is false in the app Info.plist
- [ ] Internal testers added in TestFlight; build processed and enabled
- [ ] Review notes (later public TF): Google OAuth uses `jobshout://`; SIWA on prod; agents core loop

## What this build covers

Home → Agents → **Projects** (Upwork-style freelance catalog + submit bid) → Call agent → Work → Approvals → Schedules → Me.

Projects/bids mirror jobshout.com `/projects` (same seeded catalog until the Rust freelance API is auth-wired through Go).

Not in this track yet: Showcase / Insights, chat, Career UI, APNs delivery from the server.

## Related

- Mac App Store notes: [`APP_STORE.md`](APP_STORE.md)
- Export options: [`ExportOptions-iOSAppStore.plist`](ExportOptions-iOSAppStore.plist)
- Product plan: [`docs/plans/08-ios-app.md`](../../docs/plans/08-ios-app.md)
