#!/usr/bin/env bash
# Idempotent Cloudflare CNAME upsert (beaconpulse-style).
# Env: CLOUDFLARE_API_TOKEN (required), CLOUDFLARE_ZONE or --zone
# Usage:
#   ./cloudflare-dns.sh --name int.jobshout.com --content lon1.pop0.uk --zone jobshout.com
set -euo pipefail

NAME=""
CONTENT="lon1.pop0.uk"
ZONE="${CLOUDFLARE_ZONE:-jobshout.com}"
PROXIED="false"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --name) NAME="$2"; shift 2 ;;
    --content) CONTENT="$2"; shift 2 ;;
    --zone) ZONE="$2"; shift 2 ;;
    --proxied) PROXIED="$2"; shift 2 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

if [[ -z "$NAME" ]]; then
  echo "usage: $0 --name <fqdn> [--content lon1.pop0.uk] [--zone jobshout.com]" >&2
  exit 2
fi
if [[ -z "${CLOUDFLARE_API_TOKEN:-}" ]]; then
  echo "CLOUDFLARE_API_TOKEN not set — skipping DNS for ${NAME}" >&2
  exit 0
fi

api="https://api.cloudflare.com/client/v4"
auth=(-H "Authorization: Bearer ${CLOUDFLARE_API_TOKEN}" -H "Content-Type: application/json")

zone_id=$(curl -fsS "${auth[@]}" "$api/zones?name=${ZONE}" | jq -r '.result[0].id // empty')
if [[ -z "$zone_id" ]]; then
  echo "zone ${ZONE} not found for this token — leaving DNS as-is" >&2
  exit 0
fi

# Drop conflicting A/AAAA so CNAME can land cleanly.
for typ in A AAAA; do
  while read -r rid; do
    [[ -z "$rid" ]] && continue
    curl -fsS "${auth[@]}" -X DELETE "$api/zones/$zone_id/dns_records/$rid" >/dev/null
    echo "deleted conflicting ${typ} record ${rid}"
  done < <(curl -fsS "${auth[@]}" "$api/zones/$zone_id/dns_records?type=${typ}&name=${NAME}" \
    | jq -r '.result[].id // empty')
done

rec=$(curl -fsS "${auth[@]}" "$api/zones/$zone_id/dns_records?type=CNAME&name=${NAME}")
rec_id=$(echo "$rec" | jq -r '.result[0].id // empty')
existing=$(echo "$rec" | jq -r '.result[0].content // empty')
body=$(jq -nc --arg name "$NAME" --arg content "$CONTENT" --argjson proxied "$PROXIED" \
  '{type:"CNAME",name:$name,content:$content,ttl:1,proxied:$proxied}')

if [[ -n "$rec_id" && "$existing" == "$CONTENT" ]]; then
  echo "noop CNAME ${NAME} -> ${CONTENT}"
  exit 0
fi
if [[ -n "$rec_id" ]]; then
  curl -fsS "${auth[@]}" -X PUT "$api/zones/$zone_id/dns_records/$rec_id" --data "$body" >/dev/null
  echo "updated CNAME ${NAME} -> ${CONTENT}"
else
  curl -fsS "${auth[@]}" -X POST "$api/zones/$zone_id/dns_records" --data "$body" >/dev/null
  echo "created CNAME ${NAME} -> ${CONTENT}"
fi
