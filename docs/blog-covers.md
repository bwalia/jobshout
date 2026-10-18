# Blog covers — unique listing images

Workstation listing cards (`/en/docs/blogs`) use each post's OpsAPI
`featured_image_url`. When that field is empty, the site used to fall back to
the same `/img/ai-agents-business-hero.jpg` for every JobShout-filed article.

## How JobShout draws a cover

With `BLOG_COVER_IMAGES=true` (all rings), each article run:

1. Plans a **cover brief** (`cover_metaphor`, `cover_objects`, `cover_accent`)
   grounded in the article's angle — not a generic tech collage.
2. Draws a 1536×864 cover (Gemini → mflux → OpenAI).
3. Publishes it as OpsAPI `featured_image_url` (absolute URL).

See [image-generation.md](./image-generation.md) for providers and rings.

## Regenerate from finished content

When a published piece has no cover (or a stale one), redraw from the article
body without re-running research/draft:

```http
POST /api/v1/blogs/articles/{articleID}/regenerate-cover
Authorization: Bearer …
Content-Type: application/json

{"sync_cms": true}
```

- Without `sync_cms`, only JobShout's `blog_articles.cover_image_url` updates.
- With `sync_cms: true` and a stored `post_uuid`, OpsAPI
  `featured_image_url` is updated too (needs `cms:update` on the key).

The regenerator derives a fresh metaphor from title, excerpt and body so two
neighbouring topics never share the same focal objects.

## Fix coverless live posts without a JobShout article row

1. Add a unique SVG (or PNG) at
   `workstation-website/public/img/blog/<slug>/cover.svg`.
2. Point OpsAPI at it (absolute URL the public site can load):

```bash
# OPSAPI_BASE_URL, OPSAPI_API_KEY, OPSAPI_NAMESPACE=workstation
./scripts/backfill-blog-featured-images.sh
```

The website `BlogCard` also tries `/img/blog/{slug}/cover.svg` before the
shared hero, so listing cards recover as soon as the asset is deployed even
if CMS update lags.

## Writer / agent instructions

When cloning a writer for a site ([writer-agents.md](./writer-agents.md)):

- Keep `BLOG_COVER_IMAGES` on for that ring.
- Do not put image credentials in the launch schema — covers use the ring's
  image providers.
- After publishing, if the listing still shows the shared hero, call
  `regenerate-cover` with `sync_cms: true`, or run the backfill script.
