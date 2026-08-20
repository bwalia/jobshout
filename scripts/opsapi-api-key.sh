#!/usr/bin/env bash
#
# Mint a namespace-scoped opsapi API key for the CMS pipeline and print it,
# for pasting into OPSAPI_API_KEY (.env locally, the deployment's secret
# store otherwise).
#
# The login + emailed-OTP dance below is a one-time provisioning step: it only
# obtains a short-lived admin JWT, which is then used to create the API key at
# POST /api/v2/api-keys. The key itself is a long-lived machine credential —
# the server never logs in or refreshes anything at runtime, and the key can
# be revoked at any time with DELETE /api/v2/api-keys/<uuid>.
#
# Usage:
#   ./scripts/opsapi-api-key.sh                       # prompts for everything
#   OPSAPI_BASE_URL=https://int-opsapi.workstation.co.uk \
#   OPSAPI_USER=someone@example.com \
#   OPSAPI_NAMESPACE=workstation ./scripts/opsapi-api-key.sh
#
# The logging-in user must be an admin (namespace.manage) of the namespace.
# Nothing is written to disk and no credential is passed on the command line,
# where it would land in shell history and the process table.

set -euo pipefail

BASE_URL="${OPSAPI_BASE_URL:-}"
USER_ID="${OPSAPI_USER:-}"
NAMESPACE="${OPSAPI_NAMESPACE:-}"
KEY_NAME="${OPSAPI_KEY_NAME:-jobshout-blog}"

if [[ -z "$BASE_URL" ]]; then
    read -r -p "opsapi API base URL (e.g. https://int-opsapi.workstation.co.uk): " BASE_URL
fi
BASE_URL="${BASE_URL%/}"

if [[ -z "$USER_ID" ]]; then
    read -r -p "opsapi username or email (a namespace admin): " USER_ID
fi

if [[ -z "$NAMESPACE" ]]; then
    read -r -p "opsapi namespace slug the key will belong to: " NAMESPACE
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

jwt=$(printf '%s' "$verify" | jq -r '.token // empty')
if [[ -z "$jwt" ]]; then
    echo "verification failed:" >&2
    printf '%s\n' "$verify" | jq . >&2 2>/dev/null || printf '%s\n' "$verify" >&2
    exit 1
fi

# Step 3 — mint the key. Scoped to creating CMS posts and nothing else; a
# leaked key cannot read data or touch any other module.
created=$(curl -sS -m 30 -X POST "$BASE_URL/api/v2/api-keys" \
    -H "Authorization: Bearer $jwt" \
    -H "X-Namespace-Slug: $NAMESPACE" \
    -H "Content-Type: application/json" \
    -d "$(jq -nc --arg n "$KEY_NAME" '{name:$n, scopes:{cms:["create"]}}')")

key=$(printf '%s' "$created" | jq -r '.data.key // empty')
if [[ -z "$key" ]]; then
    echo "key creation failed:" >&2
    printf '%s\n' "$created" | jq . >&2 2>/dev/null || printf '%s\n' "$created" >&2
    exit 1
fi

printf '%s' "$created" | jq -r \
    '"\nCreated API key \"\(.data.name)\" (\(.data.uuid)) in namespace '"$NAMESPACE"', scope cms:create.\nThis is the only time the key is shown. Set OPSAPI_API_KEY to it and OPSAPI_NAMESPACE to '"$NAMESPACE"'.\nRevoke it any time: DELETE '"$BASE_URL"'/api/v2/api-keys/\(.data.uuid)\n"' >&2

echo "$key"
