# iPhone TestFlight (JobShout)

Primary TestFlight track: **Release** → prod API → bundle `com.tenthmatrix.jobshout` → ASC Apple ID **`511622488`**.

| | |
|---|---|
| Display name | JobShout |
| Scheme | `JobShout` |
| Configuration | Release |
| Bundle ID | `com.tenthmatrix.jobshout` |
| App Store Connect Apple ID | `511622488` (permanent) |
| ASC URL | https://appstoreconnect.apple.com/apps/511622488/testflight/ios |
| Marketing version | `0.1.0` (`MARKETING_VERSION` in `project.yml`) |
| Build | `CURRENT_PROJECT_VERSION` (bump each upload) |
| Min iOS | 18.0 |
| Team | `PAS2QUVJHC` (pass via script/env; never commit in `project.yml`) |

## One-shot archive + upload

```sh
cd ios && JOBSHOUT_UPLOAD=1 ./scripts/release-ios-testflight.sh
```

`ASC_APPLE_ID` defaults to `511622488`. Override only if Apple assigns a new app id.

Or set `APP_STORE_CONNECT_KEY_ID`, `APP_STORE_CONNECT_ISSUER_ID`, and
`APP_STORE_CONNECT_KEY_PATH` (`.p8`). If CLI upload still fails, open the
archive in **Xcode → Organizer → Distribute App → App Store Connect → Upload**.

Note: Homebrew `rsync` 3.x breaks export (`Copy failed`); the script puts
`/usr/bin` first on `PATH` so macOS openrsync is used.

## App Store Connect / Developer Portal checklist

ASC app **`511622488`** must use bundle **`com.tenthmatrix.jobshout`** (Apple
App IDs allow letters, digits, `.` and `-` only — not `_`).

- [x] Developer Portal App ID `com.tenthmatrix.jobshout` (Push + Sign in with Apple; Store profile on team `PAS2QUVJHC`)
- [ ] ASC app `511622488` bundle set to `com.tenthmatrix.jobshout` (My Apps → app → App Information). If the bundle cannot be changed, create the app with that bundle and update `ASC_APPLE_ID` / this doc.
- [ ] Xcode signed into team `PAS2QUVJHC`
- [ ] Optional: App Store Connect API key (Issuer ID + Key ID + `.p8`) for non-interactive upload
- [ ] Prod Helm `apple.clientIds` includes `com.tenthmatrix.jobshout`
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
