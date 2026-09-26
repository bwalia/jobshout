#!/usr/bin/env bash
# Print the registry host a container on this runner should push through, or
# nothing when containers reach the LAN NodePorts directly.
#
#   usage: registry-relay.sh "192.168.1.23 192.168.1.140 ..."
#
# The runner's Docker VM (Colima) can lose its route to the LAN while the Mac
# still has one; see registry-relay.py. In that case start the relay on the
# host's loopback :30500 and report the address containers see the host at, so
# callers can put "<addr>" first in REGISTRY_HOSTS (every entry is used as
# "<addr>:30500"). BuildKit must reach it with `http = true` and WITHOUT
# `insecure = true`: with insecure it tries HTTPS first and the relay's
# plain-HTTP upstream answers that with EOF. The runner host itself cannot
# reach <addr>, so host-side registry calls keep using the LAN entries.
# Diagnostics go to stderr; only the answer goes to stdout.
set -euo pipefail

HOSTS="$1"
PORT=30500
PROBE_IMAGE="${RELAY_PROBE_IMAGE:-alpine:3}"
DIR="$(cd "$(dirname "$0")" && pwd)"
LOG="${RUNNER_TEMP:-/tmp}/registry-relay.log"

# 0 when `wget` inside a throwaway container gets any HTTP answer (401 is the
# normal unauthenticated reply) from http://$1/v2/.
container_reaches() {
  docker run --rm "$PROBE_IMAGE" sh -c \
    "wget -S -T 5 -O /dev/null 'http://$1/v2/' 2>&1 | grep -q 'HTTP/'" >/dev/null 2>&1
}

for h in $HOSTS; do
  if container_reaches "$h:$PORT"; then
    echo "registry-relay: containers reach $h:$PORT directly; no relay needed" >&2
    exit 0
  fi
done
echo "registry-relay: containers cannot reach any LAN NodePort; starting host relay" >&2

answers() { curl -s -o /dev/null -w '%{http_code}' --max-time 5 "http://127.0.0.1:$PORT/v2/" 2>/dev/null | grep -qE '^(200|401)$'; }
# Always start fresh: a relay left over from an earlier job can lose its own
# route to the LAN and then drops every connection while still listening.
pkill -f "registry-relay.py $PORT " 2>/dev/null || true
sleep 1
upstreams=""
for h in $HOSTS; do upstreams="$upstreams $h:$PORT"; done
# shellcheck disable=SC2086
nohup python3 "$DIR/registry-relay.py" "$PORT" $upstreams >>"$LOG" 2>&1 &
for _ in 1 2 3 4 5 6 7 8 9 10; do answers && break; sleep 1; done
if ! answers; then
  echo "::warning::registry relay did not come up on 127.0.0.1:$PORT (log: $LOG)" >&2
  exit 0
fi

# Colima/Lima expose the host at 192.168.5.2; Docker Desktop at host.docker.internal.
for addr in 192.168.5.2 host.docker.internal; do
  if container_reaches "$addr:$PORT"; then
    echo "registry-relay: containers reach the registry via $addr:$PORT" >&2
    echo "$addr"
    exit 0
  fi
done
echo "::warning::registry relay is up but no container-visible host address reaches it" >&2
