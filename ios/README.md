# JobShout Apple apps

Native SwiftUI clients for the JobShout platform (iPhone/iPad, Mac, Watch).
Plan and scope: [`docs/plans/08-ios-app.md`](../docs/plans/08-ios-app.md).

The iPhone app covers the core loop: sign in (Sign in with Apple or email),
browse agents, **call an agent** with its server-defined launch form, watch
the work live, and **approve or reject** what agents ask to do. Mac and Watch
share `JobShoutKit` and the same Keychain access group so a session can move
across devices of the same team.

## Layout

```
ios/
├── project.yml        XcodeGen spec (the .xcodeproj is generated, not committed)
├── App/               iPhone/iPad @main, push registration, assets
├── Mac/App/           macOS @main shell
├── Watch/App/         watchOS @main shell (independent; no WebSocket)
├── UITests/           end-to-end test against a running API
└── JobShoutKit/       Swift package: everything else, testable on macOS
    ├── JobShoutCore       environments, logging
    ├── JobShoutAPI        client generated from server/api/openapi.yaml (symlink) + auth middleware
    ├── JobShoutAuth       Keychain session, single-flight refresh, sign-in flows
    ├── JobShoutLive       WebSocket live events with reconnect/backoff (not used on watchOS)
    └── JobShoutFeatures   screens and @Observable models
```

The API client is generated at build time by `swift-openapi-generator` from
`server/api/openapi.yaml`. A Go test (`server/cmd/server/openapi_test.go`)
fails if that spec documents a route the server doesn't register, so the app
can't be built against an endpoint that doesn't exist.

Launch forms are **not** coded per agent: the app renders
`GET /agent-schemas` (or `/agent-schemas/generic` for custom agents), the same
schema the web uses. A new specialist appears in the app with no app release.

## Run

```sh
brew install xcodegen
cd ios && xcodegen generate && open JobShout.xcodeproj
```

Schemes: `JobShout` (iPhone/iPad), `JobShoutMac`, `JobShoutWatch`.

| Configuration | Ring | iPhone/iPad | Mac | Watch |
|---|---|---|---|---|
| Debug | int | `com.jobshout.app.dev` | `com.jobshout.mac.dev` | `com.jobshout.app.watchkitapp.dev` |
| Staging | acc | `com.jobshout.app.staging` | `com.jobshout.mac.staging` | `com.jobshout.app.watchkitapp.staging` |
| Release | prod | `com.jobshout.app` | `com.jobshout.mac` | `com.jobshout.app.watchkitapp` |

Set your team in Xcode (never commit `DEVELOPMENT_TEAM`). On the simulator,
ad-hoc signing is enough: `CODE_SIGN_IDENTITY=- CODE_SIGN_STYLE=Manual`.
An unsigned build (`CODE_SIGNING_ALLOWED=NO`) has no Keychain access, so
sign-in fails with "Couldn't store your sign-in securely".

**Default API for Debug is int** (`https://int.jobshout.co.uk/api/v1`). Open
the `JobShout`, `JobShoutMac`, or `JobShoutWatch` scheme, set your team, Run.
On int, prefer **Continue with Google** (OAuth is enabled there). Email/password
also works. Sign in with Apple is off on int. The footer should say "Connected
to Integration." Google uses the `jobshout://` callback scheme (`native=1` on
`/auth/google/start`); that server support must be deployed to the ring you hit.

```sh
brew install xcodegen
cd ios && xcodegen generate && open JobShout.xcodeproj
# Xcode → scheme JobShoutMac (or JobShout) → Run
# Sign in with your int.jobshout.co.uk email/password
```

CLI smoke (Mac, no signing team needed for a compile-only check):

```sh
cd ios && xcodegen generate
xcodebuild build -project JobShout.xcodeproj -scheme JobShoutMac -configuration Debug \
  -destination 'generic/platform=macOS' -skipPackagePluginValidation \
  CODE_SIGN_IDENTITY=- CODE_SIGN_STYLE=Manual DEVELOPMENT_TEAM=
```

**Against a local server:** override with `JOBSHOUT_ENVIRONMENT=local`
(and `JOBSHOUT_LOCAL_PORT`, default 8190):

```sh
SIMCTL_CHILD_JOBSHOUT_ENVIRONMENT=local SIMCTL_CHILD_JOBSHOUT_LOCAL_PORT=8190 \
  xcrun simctl launch booted com.jobshout.app.dev
# Mac: Product → Scheme → Edit Scheme → Run → Arguments → Environment Variables
#   JOBSHOUT_ENVIRONMENT=local
```

**Sign in with Apple** (Staging/Release rings) needs every bundle id above in
the ring's `APPLE_CLIENT_IDS` (Helm `apple.clientIds`) and the capability on
each App ID. Shared sessions use the Keychain access group `com.jobshout.shared`.

## Test

```sh
cd ios/JobShoutKit && swift test            # unit tests, macOS, no simulator
```

The UI test signs in and walks Home → Agents → Call agent → Work → task →
Approvals → Me against a real server. It skips unless given an account:

```sh
TEST_RUNNER_JOBSHOUT_UITEST_EMAIL=you@example.com \
TEST_RUNNER_JOBSHOUT_UITEST_PASSWORD=… \
TEST_RUNNER_JOBSHOUT_UITEST_PORT=8190 \
xcodebuild test -project JobShout.xcodeproj -scheme JobShout \
  -destination 'platform=iOS Simulator,name=iPhone 17 Pro' \
  -skipPackagePluginValidation CODE_SIGN_IDENTITY=- CODE_SIGN_STYLE=Manual DEVELOPMENT_TEAM=
```

## Rules the code keeps

- Tokens live only in the Keychain (`AfterFirstUnlockThisDeviceOnly`), never
  in UserDefaults, logs or notifications.
- Live events are hints to refetch; screens also poll while work is moving
  and always show when data was last loaded.
- Response status fields are strings, not enums, so a new server value can't
  break decoding.
- Approving an agent's action asks for Face ID / passcode first.
