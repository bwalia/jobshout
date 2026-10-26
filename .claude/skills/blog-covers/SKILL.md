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
2. Prefer JobShout pipeline over hand-edited assets. Covers default to `BLOG_COVER_STYLE=diagram` — the
   article's idea as a vector diagram (layout + labels from the plan's `cover_scene`), no GPU. JobShout never
   files a post without a verified featured image (stored cover → drawn cover → diagram/gradient fallback) —
   see `docs/blog-covers.md`.
3. Never put image credentials in launch schema fields.
4. After regenerating, sync CMS (`sync_cms: true`).
5. **Never set `featured_image_url` to a URL that does not return 200 with an image.** OpsAPI stores it verbatim
   and the live site shows a broken card. Check with `curl -sI <url>` first.

## JobShout API

```http
POST /api/v1/blogs/articles/{articleID}/regenerate-cover
{"sync_cms": true}
```

Docs: `docs/blog-covers.md`, `docs/image-generation.md`.

## Live Workstation backfill (no JobShout row) — last resort

Only for a post JobShout did not write. For anything JobShout filed, use `regenerate-cover` above.

1. Ensure `workstation-website/public/img/blog/<slug>/cover.svg` exists (unique per slug).
2. Deploy the website and confirm `/img/blog/<slug>/cover.svg` returns 200 — the script refuses to point a
   post at a URL that does not (it exits non-zero and leaves the post untouched).
3. Run with a `cms:update` key:

```bash
OPSAPI_BASE_URL=https://opsapi.workstation.co.uk \
OPSAPI_API_KEY=opsk_… \
OPSAPI_NAMESPACE=workstation \
./scripts/backfill-blog-featured-images.sh
```

`BlogCard` also falls back to `/img/blog/{slug}/cover.svg` before the shared hero.
