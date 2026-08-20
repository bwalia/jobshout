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

# Fail before the first prompt: nobody should type their password only to
# learn a dependency is missing.
command -v jq >/dev/null 2>&1 || {
    echo "error: jq is required" >&2
    exit 1
}

# die <label> <response>: report a failed step with the server's response,
# pretty-printed when it is JSON.
die() {
    echo "$1:" >&2
    printf '%s\n' "$2" | jq . >&2 2>/dev/null || printf '%s\n' "$2" >&2
    exit 1
}

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

# Step 1 — password. This route reads self.params, so it wants a form body
# rather than JSON; sending JSON gets a "identifier required" 400.
login=$(curl -sS -m 30 -X POST "$BASE_URL/auth/login" \
    --data-urlencode "username=$USER_ID" \
    --data-urlencode "password=$PASSWORD")

session=$(printf '%s' "$login" | jq -r '.session_token // empty')
[[ -n "$session" ]] || die "login failed" "$login"

echo "Password accepted. A verification code was sent to the account's email." >&2
read -r -p "Verification code: " CODE

# Step 2 — OTP. This route does parse its body as JSON.
verify=$(curl -sS -m 30 -X POST "$BASE_URL/auth/2fa/verify" \
    -H "Content-Type: application/json" \
    -d "$(jq -nc --arg s "$session" --arg c "$CODE" '{session_token:$s, code:$c}')")

jwt=$(printf '%s' "$verify" | jq -r '.token // empty')
[[ -n "$jwt" ]] || die "verification failed" "$verify"

# Step 3 — mint the key. Scoped to creating CMS posts and nothing else; a
# leaked key cannot read data or touch any other module.
created=$(curl -sS -m 30 -X POST "$BASE_URL/api/v2/api-keys" \
    -H "Authorization: Bearer $jwt" \
    -H "X-Namespace-Slug: $NAMESPACE" \
    -H "Content-Type: application/json" \
    -d "$(jq -nc --arg n "$KEY_NAME" '{name:$n, scopes:{cms:["create"]}}')")

key=$(printf '%s' "$created" | jq -r '.data.key // empty')
[[ -n "$key" ]] || die "key creation failed" "$created"

# Informational only — guarded with || true so a formatting failure can never
# suppress the key below, which is shown exactly once.
printf '%s' "$created" | jq -r --arg ns "$NAMESPACE" --arg base "$BASE_URL" \
    '"\nCreated API key \"\(.data.name)\" (\(.data.uuid)) in namespace \($ns), scope cms:create.\nThis is the only time the key is shown. Set OPSAPI_API_KEY to it and OPSAPI_NAMESPACE to \($ns).\nRevoke it any time: DELETE \($base)/api/v2/api-keys/\(.data.uuid)\n"' >&2 \
    || true

echo "$key"
