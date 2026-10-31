# iPhone TestFlight (JobShout)

Primary TestFlight track: **Release** → prod API → bundle `com.tenthmatrix.jobshout` → ASC Apple ID **`511622488`**.

| | |
|---|---|
| Display name | JobShout |
| Scheme | `JobShout` |
| Configuration | Release |
| Bundle ID | `com.tenthmatrix.jobshout` (all lowercase — ASC + Developer Portal) |
| App Store Connect Apple ID | `511622488` (permanent) |
| ASC URL | https://appstoreconnect.apple.com/apps/511622488/testflight/ios |
| Marketing version | `0.1.0` (`MARKETING_VERSION` in `project.yml`) |
| Build | `CURRENT_PROJECT_VERSION` (bump each upload) |
| Min iOS | 18.0 |
| Team | `PAS2QUVJHC` (pass via script/env; never commit in `project.yml`) |

## Fastlane (preferred)

```sh
cd ios
# Optional but recommended for non-interactive upload:
export APP_STORE_CONNECT_KEY_ID=6KVVV27G4Q   # or your Key ID
export APP_STORE_CONNECT_ISSUER_ID=<Issuer-UUID-from-ASC-Keys>
export APP_STORE_CONNECT_KEY_PATH=$HOME/.appstoreconnect/private_keys/AuthKey_${APP_STORE_CONNECT_KEY_ID}.p8

fastlane ios beta
```

Issuer ID: App Store Connect → Users and Access → Integrations → App Store Connect API.

Without an API key, Fastlane falls back to Apple ID / Xcode session upload.
`SKIP_UPLOAD=1 fastlane ios beta` builds the IPA only.

## Shell script (alternate)

```sh
cd ios && JOBSHOUT_UPLOAD=1 ./scripts/release-ios-testflight.sh
```

`ASC_APPLE_ID` defaults to `511622488`. If CLI upload still fails, open the
archive in **Xcode → Organizer → Distribute App → App Store Connect → Upload**.

Note: Homebrew `rsync` 3.x breaks export (`Copy failed`); Fastlane and the
script put `/usr/bin` first on `PATH` so macOS openrsync is used.

## App Store Connect / Developer Portal checklist

ASC Bundle ID must be selected from the **registered App ID** list as
**`com.tenthmatrix.jobshout`** (all lowercase). Do not use
`com.tenthmatrix.Jobshout` — Apple treats that as unavailable because the
lowercase App ID already exists on team `PAS2QUVJHC`.

- [x] Developer Portal App ID `com.tenthmatrix.jobshout` (Push + Sign in with Apple; Store profile on team `PAS2QUVJHC`)
- [ ] ASC app `511622488` → App Information → Bundle ID → pick **`com.tenthmatrix.jobshout`** (lowercase) from the dropdown, then Save
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
