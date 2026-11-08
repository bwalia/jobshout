# iPhone TestFlight (JobShout)

Primary TestFlight track: **Release** → prod API → bundle `com.jobshout.app` → ASC Apple ID **`6829933699`**.

| | |
|---|---|
| Display name | JobShout |
| Scheme | `JobShout` |
| Configuration | Release |
| Bundle ID | `com.jobshout.app` |
| SKU | `jobshout-ios` |
| App Store Connect Apple ID | `6829933699` (permanent) |
| ASC URL | https://appstoreconnect.apple.com/apps/6829933699/testflight/ios |
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

`ASC_APPLE_ID` defaults to `6829933699`. If CLI upload still fails, open the
archive in **Xcode → Organizer → Distribute App → App Store Connect → Upload**.

Note: Homebrew `rsync` 3.x breaks export (`Copy failed`); Fastlane and the
script put `/usr/bin` first on `PATH` so macOS openrsync is used.

## App Store Connect / Developer Portal checklist

ASC app **`6829933699`** Bundle ID must be **`com.jobshout.app`** (same as the
Developer Portal App ID + Store profile on team `PAS2QUVJHC`).

- [x] Developer Portal App ID `com.jobshout.app` (Push + Sign in with Apple; Store profile on team `PAS2QUVJHC`)
- [x] ASC app `6829933699` uses bundle `com.jobshout.app`
- [ ] Xcode signed into team `PAS2QUVJHC`
- [ ] Optional: App Store Connect API key (Issuer ID + Key ID + `.p8`) for non-interactive upload
- [ ] Prod Helm `apple.clientIds` includes `com.jobshout.app`
- [ ] Privacy: `ITSAppUsesNonExemptEncryption` is false in the app Info.plist
- [ ] Internal testers added in TestFlight; build processed and enabled
- [ ] Review notes (later public TF): Google OAuth uses `jobshout://`; SIWA on prod; agents core loop

## What this build covers

Home → Agents (richer detail: model, knowledge/skills, **Run**, New task, Ask in chat) → **Chat** (sessions + SSE stream against the ring) → **Freelance** (jobshout.com marketplace catalog + bid; deliberately not named “Projects” to avoid colliding with workspace board projects) → **Work** (WS live + keepalive + foreground resync; polling remains truth) → Approvals → Schedules (**create** + pause/resume) → Me.

Specialists with a web Workspace (`AGENT_CLIENTS`) get **Open workspace on web** → `/panel/agents/{id}?tab=workspace` (no per-builtin Swift clones). Debug builds talk to int.

Parity IDs from `docs/prompts/agents-web-ios-parity.md`: **P1–P10** shipped for Agents core loop (Phase A web + Phase B iOS). Phase C polish: Work WS reliability, Freelance naming, client push registration + tap→Approvals.

Freelance/bids mirror jobshout.com `/projects` (same seeded catalog until the Rust freelance API is auth-wired through Go).

### APNs gap (intentional leftover)

- **Client:** registers for remote notifications, uploads device push token via `PUT /devices/{id}/push`, opens Approvals on notification tap.
- **Server:** no APNs sender yet. Approval notices still go through Slack/Teams/Telegram (`notifyManager` → `notifSvc`). Device table + token storage exist; push-when-approval-needed waits on an APNs sender that reads `devices.push_token`.

## Related

- Mac App Store notes: [`APP_STORE.md`](APP_STORE.md)
- Export options: [`ExportOptions-iOSAppStore.plist`](ExportOptions-iOSAppStore.plist)
- Product plan: [`docs/plans/08-ios-app.md`](../../docs/plans/08-ios-app.md)
