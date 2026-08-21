# Image generation

JobShout can draw pictures: covers for the articles it writes, illustrations
inside them, images an agent asks for mid-task, and one-offs from the Images
page.

## Why the model runs on the workstation

Everything else in this repository is a container scheduled onto k3s1. The image
model is not, and cannot be.

The generator is [mflux](https://github.com/filipstrand/mflux), which runs on
Apple's **MLX** framework. MLX needs a Metal GPU, so it runs on Apple Silicon and
nowhere else. The k3s1 nodes are **amd64 Linux**. There is no build of this that
runs in a pod.

That is the same constraint the platform already worked around for language
models: Ollama also runs on the workstation, and every ring points at
`https://ollama.workstation.co.uk` rather than at an in-cluster service. Image
generation is deliberately built to the identical shape — one service on the
workstation, every environment pointing at it, an HS256 JWT per request.

```
  int ─┐
 test ─┼──> https://images.workstation.co.uk ──> image-service (FastAPI)
  acc ─┤         (JWT gateway)                        └─> mflux ──> Metal GPU
 prod ─┘
```

### A note on Ollama

Ollama **cannot generate images**, and this is a property of the runtime rather
than of which models are installed. Its API exposes `/api/chat`, `/api/generate`
and `/api/embed`; there is no image endpoint. The `vision` capability reported
for models like `muse-glimmer` and `minicpm-v` means the opposite of what it
sounds like — a CLIP projector that lets the model *read* an image and answer in
text. Image *output* needs a diffusion runtime, which is what mflux is.

## The pieces

| Component                        | What it does                                            |
| -------------------------------- | ------------------------------------------------------- |
| `image-service/`                 | FastAPI over mflux. Runs on the Mac. See its README.     |
| `server/internal/imagegen`       | Provider interface + Router. Mirrors `internal/llm`.     |
| `server/internal/imagestore`     | Puts PNGs in MinIO, hands back a URL.                    |
| `server/internal/service`        | `ImageService` — generate, store, record, in one place.  |
| `server/internal/tools`          | `generate_image`, the agent-callable tool.               |
| `server/internal/blog`           | The `illustrating` step: covers and in-article pictures. |
| `server/internal/handler`        | `/api/v1/images/*`.                                      |
| `web/nextjs/.../images`          | The Images page and the model picker.                    |

### Providers

**`mflux`** — the workstation service. Local weights, nothing leaves the network,
free per image, and unavailable whenever that Mac is asleep. Two models have
weights on that machine:

| Model             | 1024×576 | Notes                                          |
| ----------------- | -------- | ---------------------------------------------- |
| `z-image-turbo`   | ~40–90 s | Few-step turbo. What blog covers use.          |
| `qwen-image-2512` | ~3.5 min | Qwen-Image-2512, 4-bit MLX. Available for experiments. |

Blog covers pin `z-image-turbo` in code (`coverModel`) with a dark editorial
template, 1536×864, and eight steps. That is fast enough for retries and strong
enough at short on-image titles. Name `qwen-image-2512` explicitly only when
you want the slower Qwen path.

`qwen-image-2512` is a community 4-bit build rather than one of mflux's built-in
entries, and is deliberately distinct from mflux's `qwen-image`, which is a
different 60 GB repo that is not downloaded. See `image-service/README.md`.

**`openai`** — OpenAI's image API, using the same `OPENAI_API_KEY` as chat. Always
reachable, costs per image, and the prompt leaves the network. Registered
automatically when a key is set, so it is there as a fallback when the
workstation is not.

The Router picks the configured default and falls back to whichever provider
exists if that one did not register.

## Configuration

| Variable              | Default         | Meaning                                       |
| --------------------- | --------------- | --------------------------------------------- |
| `IMAGE_PROVIDER`      | `mflux`         | Default provider.                             |
| `IMAGE_BASE_URL`      | *(unset)*       | Workstation image service. Unset disables it. |
| `IMAGE_DEFAULT_MODEL` | `z-image-turbo` | Model when a request names none.               |
| `IMAGE_JWT_SECRET`    | *(unset)*       | Gateway secret. Unset means unsigned requests. |
| `IMAGE_TIMEOUT`       | `30m`           | Bounds one generation.                         |
| `IMAGE_OPENAI_MODEL`  | `gpt-image-1`   | Hosted image model.                            |
| `MINIO_BUCKET_IMAGES` | `images`        | Where generated PNGs are stored.               |
| `BLOG_COVER_IMAGES`   | `false`         | Whether every article run draws a cover.       |

`IMAGE_BASE_URL` has no default on purpose. Defaulting it to localhost would
make every deployed ring try to reach an image service inside its own pod and
spend the timeout finding out there isn't one.

With neither `IMAGE_BASE_URL` nor `OPENAI_API_KEY` set, image generation is
simply off: articles are still written, the `generate_image` tool is not offered
to agents, and the Images page renders a disabled control rather than an error.

### Per-ring defaults

All four rings point at `https://images.workstation.co.uk` and set
`ai.imageModel: z-image-turbo`. The host is registered on the edge behind the
JWT gateway, and an image has been drawn through that whole path — edge,
gateway, workstation GPU.

`BLOG_COVER_IMAGES` is **on in int only**. The rollout is one ring at a time:
generate an image by hand from the Images page to confirm the endpoint answers,
then set `ai.blogCoverImages: true` in that ring's values and watch a run. A
z-image-turbo cover is usually under two minutes of a single shared GPU per
article.

The chart default in `values.yaml` is also `z-image-turbo`.

### The gateway secret

`IMAGE_JWT_SECRET` follows `OLLAMA_JWT_SECRET` exactly: an HS256 JWT in the
`x-api-key` header carrying an `app: jobshout` claim, minted per request and
never cached. `server/internal/gatewayauth` is the one implementation, shared by
both.

In Vault-backed environments the ExternalSecret extracts every key at
`secret/jobshout/<env>/config`, so **add `IMAGE_JWT_SECRET` to that path** and it
flows through without a chart change.

## Running it locally

```bash
cd image-service
./run.sh                       # port 11435, no auth
```

Then set in `.env`:

```
IMAGE_BASE_URL=http://host.docker.internal:11435
```

Without MinIO configured the platform still generates images — the bytes come
back inline as base64 and the Images page displays them — but nothing is stored,
so an article cover cannot be produced. That is why `generateCover` treats an
unstorable image as a failure rather than a success with a blank URL.

## Using it

### From the dashboard

**Images** in the sidebar. Pick a model, describe the picture, choose a shape.
The result shows its seed, which is the only way to reproduce that exact image
later.

The picker marks models that are known but **not downloaded**. mflux knows about
thirty models and this machine has weights for two — `qwen-image-2512` and
`z-image-turbo`; selecting any other starts a multi-gigabyte download that looks,
from the UI, exactly like a generation that never finishes.

### From an agent

Grant the agent the `generate_image` tool in its tool permissions. It takes a
prompt, optional dimensions and an optional seed, and returns JSON with a URL
the agent can embed.

The tool is only registered when a provider is configured — an agent granted a
tool that always answers "not configured" learns nothing useful from it.

### From the API

```
GET  /api/v1/images/models     what can draw, and what is downloaded
POST /api/v1/images/generate   {prompt, provider?, model?, width?, height?, seed?}
GET  /api/v1/images            this org's generated images, newest first
GET  /api/v1/images/file/*     the stored bytes
```

`POST /generate` answers `503` when nothing is configured or the GPU is busy, and
`400` for a bad request — a busy GPU is not a bug and should not read as one.

### In article runs

With `BLOG_COVER_IMAGES=true`, a run gains an **illustrating** step between
expanding and converting. Each article gets a 16:9 cover, and the writer may
request at most one in-body illustration with an ` ```illustration ` fence.

Nothing here can fail a run. An article without a picture is a complete article;
an article thrown away because a GPU was busy is not. Failures are reported into
the run's trace and the run carries on.

## Operational notes

**Generation is serialised.** One 27 B-parameter image model peaks near 27 GB of
MLX memory. Two at once do not fit, and the failure is not a slow response but an
OOM kill that takes the service down with it. Requests queue behind a single
lock; `IMAGE_QUEUE_TIMEOUT` bounds the wait.

**The workstation runs Ollama too.** Both compete for the same unified memory.
With several Ollama models pinned resident, an image generation may evict them
or fail to allocate. `IMAGE_QUANTIZE` trades image fidelity for headroom if that
becomes a problem.

**Images are immutable.** Object keys contain a UUID, so a regenerated image is a
new key. They are served with a one-year immutable cache header.

## Deliberately not done

- **No image editing.** mflux supports inpainting, ControlNet, depth and
  img2img. The service exposes text-to-image only; the edit variants are
  filtered out of the catalogue rather than offered and then rejected.
- **No batch generation.** One image per request, because one GPU.
- **No prompt rewriting.** The house style is appended to article prompts, but
  nothing sends the user's prompt to an LLM to "improve" it first.
- **No second hostname per model.** One host, `images.workstation.co.uk`, serves
  every model, because the model is a field in the request and there is one GPU
  behind all of them. A hostname per model would be a second edge vhost, a
  second certificate and a second route, all pointing at the same process.
