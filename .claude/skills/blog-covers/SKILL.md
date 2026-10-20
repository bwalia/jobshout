---
name: blog-covers
description: Generate or regenerate unique Article Writer / Workstation blog listing covers from article content, and backfill OpsAPI featured_image_url when cards share the same hero image.
---

# Blog covers

## When to use

- Workstation `/en/docs/blogs` cards all show `/img/ai-agents-business-hero.jpg`
- A JobShout article has no `cover_image_url` / OpsAPI has no `featured_image_url`
- Operator asks to regenerate covers from finished blog content

## Rules

1. Covers are **content-derived**: metaphor from title + angle + excerpt + body, not a generic tech collage.
2. Prefer JobShout pipeline (`BLOG_COVER_IMAGES=true`) over hand-edited assets.
3. Never put image credentials in launch schema fields.
4. After regenerating, sync CMS (`sync_cms: true` or `scripts/backfill-blog-featured-images.sh`).

## JobShout API

```http
POST /api/v1/blogs/articles/{articleID}/regenerate-cover
{"sync_cms": true}
```

Docs: `docs/blog-covers.md`, `docs/image-generation.md`.

## Live Workstation backfill (no JobShout row)

1. Ensure `workstation-website/public/img/blog/<slug>/cover.svg` exists (unique per slug).
2. Deploy the website (or confirm `/img/blog/<slug>/cover.svg` returns 200).
3. Run with a `cms:update` key:

```bash
OPSAPI_BASE_URL=https://opsapi.workstation.co.uk \
OPSAPI_API_KEY=opsk_… \
OPSAPI_NAMESPACE=workstation \
./scripts/backfill-blog-featured-images.sh
```

`BlogCard` also falls back to `/img/blog/{slug}/cover.svg` before the shared hero.
