# JobShout Image Service

An HTTP front end for [mflux](https://github.com/filipstrand/mflux), so the
JobShout platform can generate images from a text prompt.

## Why this runs outside the cluster

mflux generates on Apple's [MLX](https://github.com/ml-explore/mlx) framework,
which needs a Metal GPU — it runs on Apple Silicon and nowhere else. The k3s1
nodes JobShout deploys to are amd64 Linux, so unlike every other component in
this repository, this one **cannot be containerised and scheduled onto the
cluster**. It runs natively on the workstation Mac.

That is the same shape the platform already uses for language models: Ollama
also runs on the workstation, and every ring points at
`https://ollama.workstation.co.uk` rather than at an in-cluster pod. This
service is deliberately built to sit behind that identical arrangement, so
`int`, `test`, `acc` and `prod` all reach one image generator the same way they
already reach one Ollama.

## Endpoints

| Method | Path           | Auth | Purpose                                        |
| ------ | -------------- | ---- | ---------------------------------------------- |
| `GET`  | `/health`      | no   | Liveness. Never loads a model.                 |
| `GET`  | `/api/models`  | yes  | Which image models exist, and which are local. |
| `POST` | `/api/generate`| yes  | Generate an image from a prompt.               |

`/health` is deliberately unauthenticated and deliberately does not touch the
model: a health check that can fail for two different reasons cannot be used to
diagnose either of them.

### `POST /api/generate`

```json
{
  "prompt": "a clean editorial illustration of a lighthouse, warm amber tones",
  "model": "z-image-turbo",
  "width": 1024,
  "height": 576,
  "steps": 8,
  "seed": 42,
  "guidance": null,
  "negative_prompt": ""
}
```

Responds with the PNG base64-encoded in JSON rather than as image/png bytes:

```json
{ "image_base64": "iVBOR…", "model": "z-image-turbo", "seed": 42,
  "width": 1024, "height": 576, "steps": 8, "duration_ms": 14832 }
```

JSON because the caller stores the result in object storage and records
metadata alongside it — the seed especially, since a seed is the only way to
reproduce a given image later, and a raw image body has nowhere to put one.

## Authentication

Requests are authenticated exactly as the Ollama gateway authenticates them:
an HS256 JWT in the **`x-api-key`** header (no `Bearer` prefix), carrying an
`app` claim, signed with a shared secret. `server/internal/llm/ollama_auth.go`
mints these on the Go side and `internal/imagegen` reuses that minting.

Set the secret with `IMAGE_JWT_SECRET`. **When it is unset the service accepts
every request**, which is what makes a local development run work without
ceremony — the same nil-auth-is-valid rule the Go client follows. Never leave it
unset on a host that is reachable from outside the machine.

## Running it

```bash
cd image-service
./run.sh                       # foreground, port 11435
IMAGE_JWT_SECRET=… ./run.sh    # authenticated
```

`run.sh` uses the `mflux` tool environment installed by `uv` — mflux and its MLX
dependencies are multi-gigabyte, and installing a second copy into a private
virtualenv wastes disk to no purpose. It adds only FastAPI and uvicorn, which
that environment does not already carry.

To keep it running across logins, install the launchd agent:

```bash
./install-launchd.sh           # loads com.jobshout.image-service
launchctl list | grep jobshout
```

## Models

`GET /api/models` reports every model this service will run, each marked with
whether its weights are already in the local Hugging Face cache. Only cached
models generate promptly; the rest download tens of gigabytes on first use,
which is why the distinction is reported rather than hidden. Each entry also
carries `default_steps` — what that model gets when a request names no count.

Two models have weights on this machine:

| Name              | Repo                                | Steps | 1024×576 |
| ----------------- | ----------------------------------- | ----- | -------- |
| `z-image-turbo`   | `Tongyi-MAI/Z-Image-Turbo`          | 8     | ~40 s    |
| `qwen-image-2512` | `mlx-community/Qwen-Image-2512-4bit`| 20    | ~3.5 min |

`z-image-turbo` is the service's own default — the one a bare request with no
model gets — because a caller that did not choose is better served waiting
seconds than minutes. The platform names its model explicitly, so what an
article cover uses is set by `ai.imageModel` in the Helm values, not here.

### Third-party models

`qwen-image-2512` is not one of mflux's built-in entries. It is a community
4-bit MLX build of Qwen-Image-2512, reached on the command line as
`--model mlx-community/Qwen-Image-2512-4bit --base-model qwen`; `_CUSTOM_MODELS`
in `app/models.py` is that same pair given a name. Adding another third-party
repo means adding a row there and nothing else.

It is deliberately **not** called `qwen-image`. mflux already uses that name for
`Qwen/Qwen-Image` — a different, unquantised, roughly 60 GB repo that is not on
this disk. Collapsing the two into one name would mean a request either
generating in minutes or downloading for an hour, with nothing in the request to
say which.

Because the published repo is already 4-bit, `IMAGE_QUANTIZE` is ignored for it:
quantising an already-quantised tensor does not make it smaller, only worse.

### Disabling a model on one host

The catalogue answers what mflux can run and what is on this disk. It does not
answer what *this machine* can afford to run: a model whose weights are cached
still has to fit in the GPU beside everything else the workstation is doing, and
the largest entries evict the small turbo model that nearly every request
actually wants.

`IMAGE_DISABLED_MODELS` names catalogue entries this host refuses:

```bash
IMAGE_DISABLED_MODELS=qwen-image,qwen-image-2512 ./run.sh
```

Filtering happens in `catalogue()`, which `is_known()` reads — so a disabled
model is missing from `GET /api/models` **and** rejected by `POST /api/generate`,
rather than merely hidden from the picker while still loadable by anyone who
names it directly.

The code that loads these models is untouched, so the capability stays for a host
with room for it. Matching is on the exact catalogue name, so a family with more
than one entry needs each listed: `qwen-image` (mflux's unquantised ~60 GB
Qwen/Qwen-Image) and `qwen-image-2512` (the 4-bit repo in `models.py`) are two
different downloads, and disabling either alone leaves the other reachable.

`install-launchd.sh` writes both into the plist by default, because the
workstation agent runs with `IMAGE_MAX_LOADED_MODELS=1` and a Qwen load there
costs the next cover request a cold start.

## Concurrency and memory

Generation is **serialised behind a single lock**. A 27B-parameter image model
peaks around 27 GB of MLX memory for one 512×512 image; two at once do not fit,
and the failure mode is not a slow response but an out-of-memory kill that takes
the whole service with it. Requests queue instead, and `IMAGE_QUEUE_TIMEOUT`
bounds how long a caller waits for the lock before being told the service is
busy.

MLX GPU streams are **thread-local**. Load and generate therefore run on one
dedicated worker thread for the process lifetime. Dispatching each request onto
`asyncio.to_thread`'s default multi-worker pool (without that pin) reuses a
cached model from the wrong thread and fails with
`There is no Stream(gpu, N) in current thread`.

Loaded models are cached in memory, keyed by name. `IMAGE_MAX_LOADED_MODELS`
(default 1) bounds that cache — raising it lets a second model stay resident and
is a good way to run the machine out of memory.

## Configuration

| Variable                  | Default         | Meaning                                     |
| ------------------------- | --------------- | ------------------------------------------- |
| `IMAGE_HOST`              | `0.0.0.0`       | Bind address.                                |
| `IMAGE_PORT`              | `11435`         | Bind port (11434 is Ollama's).               |
| `IMAGE_JWT_SECRET`        | *(unset)*       | Shared secret. Unset means no auth.          |
| `IMAGE_DEFAULT_MODEL`     | `z-image-turbo` | Model used when a request names none.        |
| `IMAGE_DEFAULT_STEPS`     | *(unset)*       | Steps when a request names none. Unset means the model's own — 8 for turbo, 20 otherwise. Setting it overrides every model at once. |
| `IMAGE_MAX_STEPS`         | `50`            | Ceiling; a huge step count is a denial of service against a single-GPU host. |
| `IMAGE_MAX_DIMENSION`     | `1536`          | Ceiling on width and height.                 |
| `IMAGE_QUEUE_TIMEOUT`     | `600`           | Seconds a queued request waits for the lock. |
| `IMAGE_DISABLED_MODELS`   | *(unset)*       | Comma-separated catalogue names this host refuses to run. |
| `IMAGE_MAX_LOADED_MODELS` | `1`             | How many models stay resident.               |
| `IMAGE_QUANTIZE`          | *(unset)*       | 3/4/5/6/8 — quantise on load to cut memory.  |
| `IMAGE_LOG_LEVEL`         | `info`          | Log level.                                   |
