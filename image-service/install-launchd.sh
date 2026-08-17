#!/usr/bin/env bash
# Install the image service as a launchd user agent, so it survives a logout and
# comes back after a reboot — the same expectation the workstation's Ollama has.
set -euo pipefail

cd "$(dirname "$0")"
SERVICE_DIR="$(pwd)"

LABEL="com.jobshout.image-service"
PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
LOG_DIR="$HOME/Library/Logs/jobshout"

mkdir -p "$LOG_DIR" "$HOME/Library/LaunchAgents"

# The secret is read from the environment at install time and written into the
# plist. A plist is a file on disk, so it is created 0600 below — treat it as
# credential material.
SECRET="${IMAGE_JWT_SECRET:-}"
if [[ -z "$SECRET" ]]; then
  echo "WARNING: IMAGE_JWT_SECRET is unset. The service will accept every request." >&2
  echo "         Re-run as: IMAGE_JWT_SECRET=… $0" >&2
fi

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
        <key>IMAGE_JWT_SECRET</key>
        <string>$SECRET</string>
        <key>IMAGE_PORT</key>
        <string>${IMAGE_PORT:-11435}</string>
        <key>IMAGE_DEFAULT_MODEL</key>
        <string>${IMAGE_DEFAULT_MODEL:-z-image-turbo}</string>
        <key>HOME</key>
        <string>$HOME</string>
    </dict>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>$LOG_DIR/image-service.log</string>
    <key>StandardErrorPath</key>
    <string>$LOG_DIR/image-service.err.log</string>
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
echo "  logs:   $LOG_DIR/image-service.log"
echo "  status: launchctl print gui/$(id -u)/$LABEL | head -20"
echo "  stop:   launchctl bootout gui/$(id -u)/$LABEL"
