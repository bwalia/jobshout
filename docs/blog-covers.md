# Blog covers — unique listing images

Workstation listing cards (`/en/docs/blogs`) use each post's OpsAPI
`featured_image_url`. When that field is empty, the site used to fall back to
the same `/img/ai-agents-business-hero.jpg` for every JobShout-filed article.

## The guarantee: no post without a working featured image

OpsAPI stores `featured_image_url` verbatim (no validation, `varchar(255)`) and
the website links it as-is, so whatever JobShout sends is what readers see.
Every path that files or publishes a post therefore resolves the image first
(`blog.Runner.ensureCover`):

1. the article's recorded cover — **only if its bytes are really in image
   storage** (a recorded URL whose object is gone, or any URL not in our store,
   is not trusted);
2. otherwise a cover drawn now from the article, in the ring's
   `BLOG_COVER_STYLE` (below);
3. otherwise a fallback rendered in-process — the article's **diagram**, or if
   even that fails a branded gradient (provider `fallback`). No GPU, provider
   or network.

If none of that yields an absolute URL (e.g. `FRONTEND_BASE_URL` unset) the
post is **not filed** and the run errors: a draft is one click from going live.
Going live (`PublishPost`) sends the status and the featured image in one PUT,
so a draft filed before this existed cannot go public picture-less.

A gradient cover is recorded with provider `fallback`; redraw it with
`regenerate-cover`.

### Where covers live

Images must outlive deploys and be readable from every API replica. MinIO when
`MINIO_ENDPOINT` is set; otherwise **Postgres** (`image_blobs`, migration
000070). The pod-local directory is used only when `IMAGE_STORE_DIR` is set
explicitly (single-process laptops). Until 2026-10, prod had no MinIO and wrote
covers to each pod's disk — another reason its posts had no working image.

## How JobShout draws a cover

`BLOG_COVER_STYLE` (Helm `ai.blogCoverStyle`) picks what a cover is.

### `diagram` (default)

The article's idea drawn as a vector diagram in the house style — dark navy,
one accent, a small topic tag, no title (the site prints it under the card).
Same look as the hand-drawn covers on workstation.co.uk/en/docs/blogs.

1. The plan step returns a **`cover_scene`**: one of ten layouts plus 2–6 short
   labels in the article's own terms, an optional `stat` (only a figure a
   finding states) and an `eyebrow` tag:

   | layout | pictures | labels |
   |---|---|---|
   | `flow` | steps left → right | the steps |
   | `hub` | a core joining 4–6 things | core, then the things |
   | `loop` | stages that repeat | the stages (+ a boundary) |
   | `bars` | a number falling/rising in steps | the steps (`"caching 76"` prints 76) |
   | `versus` | one change, a good and a bad edge | good, bad |
   | `ripple` | a failure spreading | legend states |
   | `tree` | candidates, weak ones pruned | what survivors become |
   | `iceberg` | a visible effect over hidden ones | visible, then hidden |
   | `hierarchy` | a policy applied to 3 units | policy, units |
   | `threshold` | a metric drifting past its limit | distribution, score, alert, record |

2. `server/internal/blog/cover_draw.go` renders it to a 1600×900 PNG in-process
   (gg + embedded Go fonts) — no GPU, provider or network; only the image store.
3. The scene JSON is kept as the cover's prompt; provider `diagram`, model
   `scene:<layout>`.

Bars only print numbers the labels carry; otherwise they show direction with
no figures, so a cover never invents data. With no plan scene (old articles,
`regenerate-cover`) the layout is derived from title/topic/excerpt and the
labels from section headings where a layout's labels are a list of parts.

Preview every layout from real posts:

```bash
cd server && COVER_PREVIEW_DIR=/tmp/covers go test ./internal/blog -run TestDiagramCover
```

### `picture`

A generated image (needs `BLOG_COVER_IMAGES=true`):

1. Plans a **cover brief** (`cover_metaphor`, `cover_objects`, `cover_accent`)
   grounded in the article's angle — not a generic tech collage.
2. Draws a 1536×864 cover (Gemini → mflux → OpenAI).
3. Publishes it as OpsAPI `featured_image_url` (absolute URL), via the
   guarantee above. If the picture cannot be drawn, the diagram is filed.

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

## Fix coverless live posts without a JobShout article row (last resort)

Never point a post at an asset that is not deployed yet: on 2026-10-23 three
posts were pointed at `/img/blog/<slug>/cover.svg` files that did not exist and
showed broken images. The script below now refuses any URL that is not 200.


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

- Covers default to `diagram`, which needs no image provider. Keep
  `BLOG_COVER_IMAGES` on for in-article figures (and for `picture` covers).
- Do not put image credentials in the launch schema — covers use the ring's
  image providers.
- After publishing, if the listing still shows the shared hero, call
  `regenerate-cover` with `sync_cms: true`, or run the backfill script.
