#!/usr/bin/env bash
#
# Mint a seed JWT for the opsapi CMS and print it, for pasting into
# OPSAPI_TOKEN (.env locally, the deployment's secret store otherwise).
#
# Why this exists: opsapi requires 2FA on every login, so no server can obtain
# a token unattended. One is minted here by hand; the server then keeps it
# alive by exchanging it at /auth/refresh before each expiry. opsapi accepts
# that exchange for 7 days past expiry, so a token only has to be reissued if
# the server has been down longer than that.
#
# Usage:
#   ./scripts/opsapi-token.sh                       # prompts for everything
#   OPSAPI_BASE_URL=https://int-opsapi.workstation.co.uk \
#   OPSAPI_USER=someone@example.com ./scripts/opsapi-token.sh
#
# Nothing is written to disk and no credential is passed on the command line,
# where it would land in shell history and the process table.

set -euo pipefail

BASE_URL="${OPSAPI_BASE_URL:-}"
USER_ID="${OPSAPI_USER:-}"

if [[ -z "$BASE_URL" ]]; then
    read -r -p "opsapi API base URL (e.g. https://int-opsapi.workstation.co.uk): " BASE_URL
fi
BASE_URL="${BASE_URL%/}"

if [[ -z "$USER_ID" ]]; then
    read -r -p "opsapi username or email: " USER_ID
fi

# -s: no echo. Never accept the password as an argument.
read -r -s -p "Password: " PASSWORD
echo

require_jq() {
    command -v jq >/dev/null 2>&1 || {
        echo "error: jq is required" >&2
        exit 1
    }
}
require_jq

# Step 1 — password. This route reads self.params, so it wants a form body
# rather than JSON; sending JSON gets a "identifier required" 400.
login=$(curl -sS -m 30 -X POST "$BASE_URL/auth/login" \
    --data-urlencode "username=$USER_ID" \
    --data-urlencode "password=$PASSWORD")

session=$(printf '%s' "$login" | jq -r '.session_token // empty')
if [[ -z "$session" ]]; then
    echo "login failed:" >&2
    printf '%s\n' "$login" | jq . >&2 2>/dev/null || printf '%s\n' "$login" >&2
    exit 1
fi

echo "Password accepted. A verification code was sent to the account's email." >&2
read -r -p "Verification code: " CODE

# Step 2 — OTP. This route does parse its body as JSON.
verify=$(curl -sS -m 30 -X POST "$BASE_URL/auth/2fa/verify" \
    -H "Content-Type: application/json" \
    -d "$(jq -nc --arg s "$session" --arg c "$CODE" '{session_token:$s, code:$c}')")

token=$(printf '%s' "$verify" | jq -r '.token // empty')
if [[ -z "$token" ]]; then
    echo "verification failed:" >&2
    printf '%s\n' "$verify" | jq . >&2 2>/dev/null || printf '%s\n' "$verify" >&2
    exit 1
fi

# Report the namespace the token carries: publishing is scoped to it, and a
# token minted for the wrong one fails later with a confusing 403/404.
printf '%s' "$token" | jq -Rr 'split(".")[1] | @base64d' 2>/dev/null \
    | jq -r '"\nToken is for user \(.userinfo.email // .userinfo.uuid), namespace \(.userinfo.namespace.slug // "(none)"), expiring \(.exp | todate).\nSet OPSAPI_NAMESPACE to that namespace slug.\n"' >&2 \
    || true

echo "$token"
