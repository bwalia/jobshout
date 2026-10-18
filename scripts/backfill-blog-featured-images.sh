#!/usr/bin/env bash
# Backfill OpsAPI featured_image_url for Workstation posts that still use the
# shared listing hero. Points each post at
#   https://www.workstation.co.uk/img/blog/<slug>/cover.svg
# after those assets exist in workstation-website.
#
# Requires: OPSAPI_BASE_URL, OPSAPI_API_KEY, OPSAPI_NAMESPACE (default workstation)
# Optional: SITE_ORIGIN (default https://www.workstation.co.uk)
#           SLUGS="slug1 slug2" to limit the set; otherwise the six Oct-2026 posts.
set -euo pipefail

BASE="${OPSAPI_BASE_URL:?set OPSAPI_BASE_URL e.g. https://opsapi.workstation.co.uk}"
KEY="${OPSAPI_API_KEY:?set OPSAPI_API_KEY (cms:update)}"
NS="${OPSAPI_NAMESPACE:-workstation}"
SITE="${SITE_ORIGIN:-https://www.workstation.co.uk}"
BASE="${BASE%/}"

DEFAULT_SLUGS=(
  beyond-unit-tests-ensuring-reliability-in-complex-systems
  ai-s-double-edged-impact-on-devops
  gateway-api-shifts-cluster-networking
  agentic-ai-in-production
  mcp-driven-devops-boost
  aligning-workload-resilience-with-aws-resilience-hub
)

if [[ -n "${SLUGS:-}" ]]; then
  # shellcheck disable=SC2206
  TARGETS=($SLUGS)
else
  TARGETS=("${DEFAULT_SLUGS[@]}")
fi

echo "Listing published posts in namespace ${NS}..."
LIST=$(curl -fsS "${BASE}/api/v2/public/cms/${NS}/posts?page=1&perPage=50")

for slug in "${TARGETS[@]}"; do
  uuid=$(python3 -c '
import json,sys
slug=sys.argv[1]
data=json.load(sys.stdin)
for p in data.get("data") or []:
  if p.get("slug")==slug:
    print(p.get("uuid") or "")
    break
' "$slug" <<<"$LIST")
  if [[ -z "$uuid" ]]; then
    echo "skip ${slug} - not found in public CMS list"
    continue
  fi
  url="${SITE}/img/blog/${slug}/cover.svg"
  code=$(curl -sI -o /dev/null -w "%{http_code}" "$url" || true)
  if [[ "$code" != "200" ]]; then
    echo "warn ${slug} - ${url} returned HTTP ${code} (deploy the SVG first)"
  fi
  echo "PUT ${slug} -> ${url}"
  curl -fsS -X PUT "${BASE}/api/v2/cms/posts/${uuid}" \
    -H "Authorization: Bearer ${KEY}" \
    -H "Content-Type: application/json" \
    -H "Accept: application/json" \
    -H "X-Namespace-Slug: ${NS}" \
    -d "{\"featured_image_url\":$(python3 -c 'import json,sys; print(json.dumps(sys.argv[1]))' "$url")}" \
    >/dev/null
  echo "  ok (${uuid})"
done

echo "Done. Revalidate the website CMS cache if listings still show the hero."
