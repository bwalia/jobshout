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

`GET /api/models` reports every model mflux can run, each marked with whether
its weights are already in the local Hugging Face cache. Only cached models
generate promptly; the rest download tens of gigabytes on first use, which is
why the distinction is reported rather than hidden.

The default is **`z-image-turbo`** (`Tongyi-MAI/Z-Image-Turbo`): it is a
few-step turbo model, so it produces a 1024×576 cover in seconds rather than
minutes, and it is the model whose weights are already on this machine.

## Concurrency and memory

Generation is **serialised behind a single lock**. A 27B-parameter image model
peaks around 27 GB of MLX memory for one 512×512 image; two at once do not fit,
and the failure mode is not a slow response but an out-of-memory kill that takes
the whole service with it. Requests queue instead, and `IMAGE_QUEUE_TIMEOUT`
bounds how long a caller waits for the lock before being told the service is
busy.

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
| `IMAGE_DEFAULT_STEPS`     | `8`             | Steps when a request names none.             |
| `IMAGE_MAX_STEPS`         | `50`            | Ceiling; a huge step count is a denial of service against a single-GPU host. |
| `IMAGE_MAX_DIMENSION`     | `1536`          | Ceiling on width and height.                 |
| `IMAGE_QUEUE_TIMEOUT`     | `600`           | Seconds a queued request waits for the lock. |
| `IMAGE_MAX_LOADED_MODELS` | `1`             | How many models stay resident.               |
| `IMAGE_QUANTIZE`          | *(unset)*       | 3/4/5/6/8 — quantise on load to cut memory.  |
| `IMAGE_LOG_LEVEL`         | `info`          | Log level.                                   |
