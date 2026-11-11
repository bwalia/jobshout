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

### `picture` (chart default)

An infographic cover drawn by an image model that captions (needs
`BLOG_COVER_IMAGES=true`): its eyebrow tag and the same `cover_scene` as the
diagram, drawn as one labelled diagram with an icon per label, plus the stat if
there is one. No title: the site prints it under the card, and ERNIE misspelt
a 7-word one ("Premptive", "Kubernates"). The house style (deep navy, one
accent from `cover_accent`, flat icons) is fixed in `infographicCoverPrompt`,
so covers read as one series.

`imagegen.Letters(provider, model, need)` decides who may draw what:
`TextCaptions` (a cover's few labels) needs Gemini, OpenAI or ERNIE-Image;
`TextLabels` (a figure's table or flowchart) needs Gemini or OpenAI.

1. Gemini (`IMAGE_GEMINI_MODEL`, Nano Banana Pro by default) when
   `GEMINI_API_KEY` is set. Image output has no free tier: the key needs billing.
2. Otherwise, or when Gemini fails, the workstation, but only with a model that
   captions (ERNIE-Image, Apache-2.0, close to Nano Banana 2 on LongText-Bench). Every ring sets `ai.imageModel: ernie-image-turbo`; its
   weights (22 GB, no prompt enhancer) are in the workstation's HF cache. About
   6 minutes a cover beside Ollama. ERNIE letters a picture right about two
   times in three, so every cover and figure it draws is read back (Apple
   Vision OCR, `image-service/app/textcheck.py`) against the words it must
   show; a missing, misspelt, repeated or extra word gets it redrawn with a new
   seed (`IMAGE_TEXT_ATTEMPTS`, default 2), and a picture still wrong is
   refused, so the diagram of the same scene is filed instead — on a redraw
   too, rather than keeping the old cover. The prompt holds
   only words meant for the cover: a diffusion model letters whatever it reads.
   With `z-image-turbo` this step is skipped.
3. With neither, or when drawing fails, the diagram is filed: a cover whose
   labels are gibberish is worse than the diagram.

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

The regenerator derives a fresh scene from title, excerpt and body.

### Redraw all of an article's images

The article page's **Redraw images** button redraws the in-article figures and
the cover together, and updates the CMS post's body and featured image when
the article has been filed:

```http
POST /api/v1/blogs/articles/{articleID}/redraw-images
{"sync_cms": true}
```

One structured-model call reads the whole article and plans both:

- the cover: the layout whose meaning matches the article's central claim (flow
  only when the point really is a sequence of steps), labelled with the
  article's own terms. Labels that share no word with the article are dropped,
  and a stat stays only when the article states it; with fewer than two labels
  left the cover falls back to the heuristic scene;
- the one or two sections a figure teaches most in, as lists of labels, drawn
  with a lettering model if one is configured, otherwise as diagrams.

A planned figure the article does not support (copied labels, `key: value`
pairs) is planned once more, told why. Nothing is lost when drawing fails: the
article keeps its figures if no new one is drawn — less any it does not
support — and its cover if no new one can be stored. The answer is
`{"article", "notes", "figures_drawn", "cover_redrawn"}`; the page reports
success only for what changed, and `notes` say why anything did not (a full
image store, a refused CMS update).

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

- Covers are `picture` on the chart and fall back to `diagram`, which needs no
  image provider. In-article figures, in order: a hosted model that letters
  labels (Gemini or OpenAI) draws the full figure; a flow or hub figure is then
  drawn by a model that captions (ERNIE-Image) with the cover's prompt, its
  scene's labels each once (the full figure prompt made ERNIE repeat and
  misspell boxes); otherwise it is drawn as a vector diagram like the covers:
  flow and process figures as steps, architecture and concept figures as a hub.
  Comparison figures need a lettering model (a comparison belongs in a table
  otherwise). A figure is dropped when two or more labels name something the
  article never mentions (a label's words of 4+ letters must be in it — models
  copy the prompt's example figure), and a list of `key: value` pairs is not drawn as a
  hub or flow. `BLOG_COVER_IMAGES` turns on the hosted model and `picture` covers.
- Do not put image credentials in the launch schema — covers use the ring's
  image providers.
- After publishing, if the listing still shows the shared hero, call
  `regenerate-cover` with `sync_cms: true`, or run the backfill script.
