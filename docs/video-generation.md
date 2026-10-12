# Video generation (Video Producer)

The **Video Producer** agent turns a topic into a narrated video. You give it a
topic, a length and a format. It writes the script, then the workstation draws,
animates, voices, captions and edits it into an MP4 that lands on the board task.

Every model is open-weight and runs locally: nothing leaves the network, and a
video costs workstation time, not money.

## How a run works

```
 int/test/acc/prod (Go)                       workstation (image-service, Mac Studio)
 ─────────────────────                        ───────────────────────────────────────
 1. script: platform LLM → title + shots ─┐
    (narration + a picture per shot)      │   POST /api/videos  (JWT gateway,
                                          └──> images.workstation.co.uk)
                                                2. narration   Kokoro-82M  (mlx-audio)
                                                3. keyframes   z-image-turbo (mflux)
                                                4. motion      LTX-2.3 distilled (ltx-2-mlx)
 6. poll ◄──── GET /api/videos/{id} ───────     5. assembly    ffmpeg: captions, fades, concat
 7. fetch MP4 → MinIO → link on the board task
```

- **The narration sets the timing.** Each shot lasts as long as its line of
  narration, plus a short pause. Pictures follow the voice; the voice is never
  squeezed to fit the pictures.
- **Characters stay consistent** because the script describes a recurring
  person or place with the same details every time it appears. The script
  prompt requires this.
- **Motion can only help.** If a shot's animation fails, or LTX is not
  installed, that shot uses a slow camera move over its still. The run carries
  on, and the board says which shots fell back.

The pipeline lives in `image-service/app/video.py`, and the agent in
`server/internal/video/`.

## Why it renders inside the image service

Video needs the same GPU as images, and that GPU is not in the cluster (MLX is
Apple Silicon only — see [image-generation.md](image-generation.md)). Putting
video in the same process means:

- **One GPU lock.** It is taken per clip, so a cover image waits for the current
  clip (minutes), not the whole video (an hour), and the two never load models
  side by side and run out of memory.
- **No new infrastructure.** Rings reach video over the existing
  `images.workstation.co.uk` gateway with the existing `IMAGE_JWT_SECRET`.
  A second service would need its own edge vhost, router port-forward and
  secret, all kept in sync by hand.

## Using it

Open **Task Manager → Video Producer** (or ask in chat: "make a 60-second video
about …").

| Field | Notes |
| --- | --- |
| Topic | Required. One sentence is enough. |
| Length | 30 s – 3 min. About 2.5 words of narration per second, one shot per ~6 s. |
| Format | Landscape 16:9, or portrait 9:16 for Shorts, Reels and TikTok. |
| Pictures | **Animated** (LTX, ~5 min of GPU per shot) or **stills** with camera moves (~1.5 min per shot). |
| Voice | Kokoro voices in US English, UK English and Hindi. The voice also picks the script's language. |
| Style | Appended to every shot, so the whole video looks the same. |
| Captions | Burned in, timed to the narration. |

The task first gets the script and an estimate. When the video is ready, it gets
a link and moves to **done**. A one-minute animated video is about 45 minutes;
the same video as stills is about 15.

## Measured on the workstation (M4 Max, 128 GB)

| Stage | Time | Peak memory |
| --- | --- | --- |
| Script (platform LLM) | 1–3 min | (Ollama) |
| Narration, per shot (Kokoro, warm) | ~6 s | < 1 GB |
| Keyframe, 1024×576, 8 steps (z-image-turbo) | ~80 s | ~29 GB |
| Stills video, 3 shots end to end | 4 min 40 s | — |

## Operating it

**Install or repair the tools on the workstation** (idempotent):

```bash
cd image-service && ./install-video-tools.sh
```

This installs ffmpeg-full, a narration venv (mlx-audio 0.5.8 + Kokoro, with
every offered voice pre-fetched) and an LTX venv (ltx-2-mlx pinned to a tested
commit). It downloads only the weights the distilled pipeline loads (~30 GB for
LTX-2.3).

**Check what the workstation can do:** `GET /api/videos/capabilities`. When you
launch, the agent runs the same check and refuses up front if ffmpeg or
narration is missing, instead of failing an hour later.

**Upgrade to LTX-2.5** (better motion, gated weights):
1. Accept the licence at huggingface.co/dgrauet/ltx-2.5-mlx-q8 (free under $10M
   annual revenue).
2. On the workstation, run `~/.jobshout/media-tools/ltx-2-mlx/.venv/bin/hf auth login`.
3. Run `VIDEO_LTX_MODEL=dgrauet/ltx-2.5-mlx-q8 ./install-video-tools.sh`.
4. Add `VIDEO_LTX_MODEL=dgrauet/ltx-2.5-mlx-q8` to the image-service launchd
   plist's EnvironmentVariables and restart the service.

**Restarts are safe on both sides.**
- **Workstation restart:** the service re-queues interrupted jobs (at most twice)
  and keeps finished ones for 7 days.
- **API restart:** 30 s after boot, the server lists the workstation's jobs,
  finds the ones whose board task is still in progress, and resumes following
  them. Every ring shares the workstation, so each ring only resumes tasks in
  its own database.

## Configuration

Go server (all optional):

| Variable | Default | Meaning |
| --- | --- | --- |
| `VIDEO_MODEL` | provider default | LLM model that writes scripts (per-agent model settings also apply). |
| `VIDEO_MAX_RUNTIME` | `4h` | Deadline for one run, from script to stored file. |
| `VIDEO_POLL_INTERVAL` | `20s` | How often a render's progress is checked. |

The agent reuses `IMAGE_BASE_URL`, `IMAGE_JWT_SECRET`, MinIO and
`FRONTEND_BASE_URL`. If `IMAGE_BASE_URL` is unset, the agent reports itself as
not configured.

Workstation settings are listed in
[image-service/README.md](../image-service/README.md#video).

## Deliberately not done (yet)

- **Background music.** ACE-Step 1.5 (MIT) is the candidate. Narration-only
  videos are complete without it.
- **Lip-synced presenters.** No talking-head model that runs well on Apple
  Silicon passes licence review today.
- **Character reference images.** The FLUX.2 klein edit model in mflux could
  condition every shot on one character sheet. Today consistency comes from the
  script's wording, which holds well for one or two recurring subjects.
- **A dedicated tab.** The schema form, the board task and the stored MP4 cover
  it.
