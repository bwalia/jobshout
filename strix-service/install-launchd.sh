#!/usr/bin/env bash
# Install the pentest service as a launchd user agent, so it survives a logout
# and comes back after a reboot — the same expectation Ollama and the image
# service have on this machine.
set -euo pipefail

cd "$(dirname "$0")"
SERVICE_DIR="$(pwd)"

LABEL="com.jobshout.pentest-service"
PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
LOG_DIR="$HOME/Library/Logs/jobshout"

mkdir -p "$LOG_DIR" "$HOME/Library/LaunchAgents"

# Both of these are read from the environment at install time and written into
# the plist, which is a file on disk — it is created 0600 below, and the secret
# should be treated as credential material.
SECRET="${STRIX_JWT_SECRET:-}"
ALLOWLIST="${STRIX_TARGET_ALLOWLIST:-}"

if [[ -z "$SECRET" ]]; then
  echo "WARNING: STRIX_JWT_SECRET is unset. The service will accept every request." >&2
  echo "         Re-run as: STRIX_JWT_SECRET=… STRIX_TARGET_ALLOWLIST=… $0" >&2
fi
if [[ -z "$ALLOWLIST" ]]; then
  echo "WARNING: STRIX_TARGET_ALLOWLIST is unset. Every scan will be refused." >&2
  echo "         That is the safe default — set it to the hosts you are authorised" >&2
  echo "         to test, e.g. STRIX_TARGET_ALLOWLIST=juice.internal,127.0.0.1" >&2
fi

# PATH is set explicitly because a launchd agent inherits a minimal one — the
# usual reason a service that works perfectly in a shell cannot find `strix` or
# `docker` once it is an agent.
AGENT_PATH="${STRIX_AGENT_PATH:-$HOME/.local/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin}"

cat > "$PLIST" <<PLIST_EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>$LABEL</string>
    <key>ProgramArguments</key>
    <array>
        <string>$SERVICE_DIR/run.sh</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>$AGENT_PATH</string>
        <key>HOME</key>
        <string>$HOME</string>
        <key>STRIX_JWT_SECRET</key>
        <string>$SECRET</string>
        <key>STRIX_TARGET_ALLOWLIST</key>
        <string>$ALLOWLIST</string>
        <key>STRIX_PORT</key>
        <string>${STRIX_PORT:-11436}</string>
        <key>STRIX_LLM</key>
        <string>${STRIX_LLM:-ollama_chat/qwen3-coder:30b}</string>
        <key>STRIX_LLM_API_BASE</key>
        <string>${STRIX_LLM_API_BASE:-http://localhost:11434}</string>
        <key>STRIX_RUNS_DIR</key>
        <string>${STRIX_RUNS_DIR:-$SERVICE_DIR/strix_runs}</string>
        <key>STRIX_MAX_CONCURRENT</key>
        <string>${STRIX_MAX_CONCURRENT:-1}</string>
        <key>STRIX_MAX_RUNTIME_SECONDS</key>
        <string>${STRIX_MAX_RUNTIME_SECONDS:-7200}</string>
    </dict>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>$LOG_DIR/pentest-service.log</string>
    <key>StandardErrorPath</key>
    <string>$LOG_DIR/pentest-service.err.log</string>
    <key>WorkingDirectory</key>
    <string>$SERVICE_DIR</string>
</dict>
</plist>
PLIST_EOF

chmod 600 "$PLIST"

# bootout before bootstrap so re-running this picks up a changed plist rather
# than silently keeping the loaded one.
launchctl bootout "gui/$(id -u)/$LABEL" 2>/dev/null || true
launchctl bootstrap "gui/$(id -u)" "$PLIST"

echo "Installed $LABEL"
echo "  logs:   $LOG_DIR/pentest-service.log"
echo "  status: launchctl print gui/$(id -u)/$LABEL | head -20"
echo "  stop:   launchctl bootout gui/$(id -u)/$LABEL"
