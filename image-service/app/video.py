"""Narrated video rendering: a list of shots in, one MP4 out.

The platform writes the script. This module turns it into a film on the
workstation GPU, one stage at a time:

  narration  Kokoro TTS (mlx-audio), one clip per shot. Its length sets the
             shot's length, so pictures follow the voice rather than the voice
             being squeezed into pictures.
  keyframes  the image model this service already serves, one still per shot.
  motion     LTX-2 (ltx-2-mlx) animates each still. Optional, and never fatal:
             a shot whose animation fails, or a job that asks for none, gets a
             slow camera move over the still instead. Motion can only make a
             video better; it cannot fail one.
  assembly   ffmpeg: captions burned in, a short fade at each cut, concat.

Jobs take minutes to an hour, so the API is submit-and-poll, and a job's state
lives on disk next to its files so a restart neither loses a finished video nor
forgets a queued one. One job renders at a time. The GPU lock in generator.py is
shared: a cover image requested mid-render waits for the current clip, not for
the whole video.

TTS and LTX run as subprocesses from their own virtualenvs. They pin MLX
versions that differ from mflux's, and a process that exits hands every byte
back to the OS — which a 30 GB video model must do before the next still is
drawn.
"""

from __future__ import annotations

import base64
import json
import logging
import math
import os
import queue
import re
import shutil
import subprocess
import threading
import time
import uuid
from pathlib import Path

from app import config, models
from app.generator import generator

logger = logging.getLogger(__name__)

FPS = 24

# (model width, model height, output width, output height). Models render at a
# size they are quick at — keyframes need multiples of 16, LTX multiples of 32 —
# and ffmpeg scales to a frame size every player expects.
FORMATS = {
    "landscape": (1024, 576, 1280, 720),
    "portrait": (576, 1024, 720, 1280),
}

# A few Kokoro voices worth offering. The first letter is the language Kokoro
# speaks them in (a: US English, b: UK English, h: Hindi), so the voice alone
# picks the pronunciation.
VOICES = ["af_heart", "af_bella", "am_michael", "am_fenrir", "bf_emma", "bm_george", "hf_alpha", "hm_omega"]

QUEUED, RUNNING, DONE, FAILED = "queued", "running", "done", "failed"

# Padding after each line of narration, so one sentence does not run straight
# into the next shot's first word.
SHOT_TAIL = 0.5
AUDIO_LEAD = 0.15
MIN_SHOT = 2.0
FADE = 0.25

_ID = re.compile(r"^[A-Za-z0-9-]{8,64}$")
_VOICE = re.compile(r"^[a-z]{2}_[a-z]+$")


class RenderError(Exception):
    """A stage failed in a way worth showing the caller as-is."""


class QueueFull(Exception):
    """Too many jobs already waiting for the GPU."""


def valid_id(job_id: str) -> bool:
    return bool(_ID.match(job_id))


def valid_voice(voice: str) -> bool:
    return bool(_VOICE.match(voice))


def clip_frames(seconds: float) -> int:
    """Frames for an LTX clip covering `seconds`: on LTX's 8n+1 grid, within bounds."""
    frames = 8 * round(seconds * FPS / 8) + 1
    return max(25, min(frames, config.VIDEO_MAX_CLIP_FRAMES))


def shot_seconds(narration_seconds: float) -> float:
    return max(MIN_SHOT, AUDIO_LEAD + narration_seconds + SHOT_TAIL)


def caption_cues(text: str, start: float, duration: float, max_words: int = 7) -> list[tuple[float, float, str]]:
    """Split narration into short on-screen lines, timed across its audio.

    Kokoro returns no word timestamps. For steady narration, character count
    tracks speaking time closely enough that a line is never more than a word
    or two out, which reads as in sync.
    """
    words = text.split()
    if not words or duration <= 0:
        return []
    chunks: list[str] = []
    current: list[str] = []
    for word in words:
        current.append(word)
        # Break at the size limit, or early at punctuation once the line is
        # long enough not to flash past.
        if len(current) >= max_words or (word[-1] in ".,;:!?" and len(current) >= 3):
            chunks.append(" ".join(current))
            current = []
    if current:
        chunks.append(" ".join(current))

    total = sum(len(c) for c in chunks)
    cues = []
    t = start
    for chunk in chunks:
        d = duration * len(chunk) / total
        cues.append((t, t + d, chunk))
        t += d
    return cues


def srt(cues: list[tuple[float, float, str]]) -> str:
    def stamp(seconds: float) -> str:
        ms = int(round(seconds * 1000))
        h, ms = divmod(ms, 3_600_000)
        m, ms = divmod(ms, 60_000)
        s, ms = divmod(ms, 1000)
        return f"{h:02}:{m:02}:{s:02},{ms:03}"

    return "\n".join(f"{i}\n{stamp(a)} --> {stamp(b)}\n{text}\n" for i, (a, b, text) in enumerate(cues, 1))


def ken_burns(index: int, frames: int, width: int, height: int) -> str:
    """A slow zoom over a still, alternating in and out so cuts do not repeat.

    The still is upscaled first: zoompan moves in whole pixels, so on a
    frame-sized source a slow zoom visibly judders. Four times larger makes each
    step sub-pixel.
    """
    if index % 2 == 0:
        zoom = "min(zoom+0.0006,1.12)"
    else:
        zoom = "if(lte(on,1),1.12,max(zoom-0.0006,1.0))"
    return (
        f"scale={width * 4}:{height * 4},"
        f"zoompan=z='{zoom}':x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)'"
        f":d={frames}:s={width}x{height}:fps={FPS}"
    )


def caption_style(portrait: bool) -> str:
    # SRT is rendered on a 384x288 canvas scaled to the frame's height, so the
    # same number is far larger relative to a portrait frame's width.
    size = 11 if portrait else 17
    # Shorts, Reels and TikTok lay their own buttons and caption over the
    # bottom fifth of a portrait video; captions sit above that.
    margin = 60 if portrait else 26
    return (
        f"FontName=Helvetica,FontSize={size},Bold=1,PrimaryColour=&H00FFFFFF,"
        "OutlineColour=&H00000000,BackColour=&H80000000,BorderStyle=1,Outline=1.6,"
        f"Shadow=0,Alignment=2,MarginV={margin}"
    )


def _exe(path: str) -> bool:
    return bool(path) and os.path.isfile(path) and os.access(path, os.X_OK)


def capabilities() -> dict:
    motion = _exe(config.VIDEO_LTX_BIN)
    return {
        "ffmpeg": _exe(config.VIDEO_FFMPEG) and _exe(config.VIDEO_FFPROBE),
        "narration": _exe(config.VIDEO_TTS_BIN),
        "motion": motion,
        "motion_model": config.VIDEO_LTX_MODEL if motion else "",
        "keyframe_model": config.VIDEO_KEYFRAME_MODEL,
        "voices": VOICES,
        "default_voice": config.VIDEO_DEFAULT_VOICE,
        "max_shots": config.VIDEO_MAX_SHOTS,
    }


def _tool_env() -> dict:
    env = dict(os.environ)
    # Weights are installed ahead of time by install-video-tools.sh. Offline, a
    # missing model fails in a second with a clear error instead of starting a
    # 30 GB download inside somebody's render.
    env["HF_HUB_OFFLINE"] = "1"
    return env


def _run(cmd: list[str], *, timeout: float, cwd: Path | None = None) -> str:
    """Run a tool; return its combined output, or raise with the end of it."""
    try:
        proc = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, timeout=timeout, env=_tool_env())
    except subprocess.TimeoutExpired as exc:
        raise RenderError(f"{Path(cmd[0]).name} did not finish within {timeout:.0f}s") from exc
    output = f"{proc.stdout}\n{proc.stderr}".strip()
    if proc.returncode != 0:
        raise RenderError(f"{Path(cmd[0]).name} exited {proc.returncode}: {output[-600:]}")
    return output


def probe_seconds(path: Path) -> float:
    out = subprocess.run(
        [config.VIDEO_FFPROBE, "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", str(path)],
        capture_output=True, text=True, timeout=60,
    )
    try:
        return float(out.stdout.strip())
    except ValueError as exc:
        raise RenderError(f"could not read the length of {path.name}") from exc


class _Render:
    """One job's pipeline. Files live in the job's directory; nothing is shared."""

    def __init__(self, job: dict, workdir: Path, report) -> None:
        req = job["request"]
        self.job = job
        self.dir = workdir
        self.report = report
        self.shots = req["shots"]
        self.style = req.get("style", "").strip()
        self.voice = req.get("voice") or config.VIDEO_DEFAULT_VOICE
        self.captions = req.get("captions", True)
        self.motion = req.get("motion", True) and _exe(config.VIDEO_LTX_BIN)
        self.portrait = req.get("format") == "portrait"
        self.mw, self.mh, self.ow, self.oh = FORMATS["portrait" if self.portrait else "landscape"]
        seed = req.get("seed", -1)
        self.seed = seed if seed >= 0 else int(uuid.uuid4().int % (2**31 - 1))
        self.notes: list[str] = []

    def _progress(self, start: float, end: float, i: int, stage: str, message: str) -> None:
        n = len(self.shots)
        self.report(stage=stage, message=message, progress=round(start + (end - start) * i / n, 3))

    def run(self) -> Path:
        if not (_exe(config.VIDEO_FFMPEG) and _exe(config.VIDEO_FFPROBE)):
            raise RenderError(f"ffmpeg is not installed at {config.VIDEO_FFMPEG} — run install-video-tools.sh")
        if not _exe(config.VIDEO_TTS_BIN):
            raise RenderError(f"narration (mlx-audio) is not installed at {config.VIDEO_TTS_BIN} — run install-video-tools.sh")

        seconds = self.narrate()
        self.keyframes()
        clips = self.animate(seconds) if self.motion else [None] * len(self.shots)
        return self.assemble(seconds, clips)

    def narrate(self) -> list[float]:
        lengths = []
        for i, shot in enumerate(self.shots):
            self._progress(0.0, 0.05, i, "narration", f"Recording narration {i + 1} of {len(self.shots)}")
            out = self.dir / f"voice_{i:02d}"
            shutil.rmtree(out, ignore_errors=True)
            out.mkdir()
            text = shot["narration"]
            if self.voice.startswith("h"):
                # Kokoro's Hindi front end (espeak) does not chunk, and
                # truncates long input; one sentence per line it handles.
                text = re.sub(r"([।.!?])\s+", "\\1\n", text)
            said = _run([
                config.VIDEO_TTS_BIN, "--model", config.VIDEO_TTS_MODEL,
                "--voice", self.voice, "--lang_code", self.voice[0],
                "--text", text, "--output_path", str(out),
                "--file_prefix", "voice", "--audio_format", "wav", "--join_audio",
            ], timeout=600)
            wavs = sorted(out.glob("*.wav"))
            if not wavs:
                # mlx-audio reports a model or voice it cannot load and still
                # exits 0, so its output is the only place the reason is.
                raise RenderError(f"narration for shot {i + 1} produced no audio: {said[-400:]}")
            wav = self.dir / f"voice_{i:02d}.wav"
            if len(wavs) == 1:
                wavs[0].replace(wav)
            else:
                # Older mlx-audio ignores --join_audio and writes one file per
                # sentence; join them here so a shot has one narration track.
                listing = out / "list.txt"
                listing.write_text("".join(f"file '{w.name}'\n" for w in wavs))
                _run([config.VIDEO_FFMPEG, "-y", "-f", "concat", "-safe", "0", "-i", "list.txt", str(wav)],
                     timeout=120, cwd=out)
            shutil.rmtree(out, ignore_errors=True)
            lengths.append(probe_seconds(wav))
        return lengths

    def _visual(self, shot: dict) -> str:
        visual = shot["visual"].strip().rstrip(".")
        return f"{visual}. {self.style}" if self.style else visual

    def keyframes(self) -> None:
        name = config.VIDEO_KEYFRAME_MODEL
        end = 0.35 if self.motion else 0.80
        for i, shot in enumerate(self.shots):
            self._progress(0.05, end, i, "keyframes", f"Drawing shot {i + 1} of {len(self.shots)}")
            result = generator.generate(
                prompt=self._visual(shot), model_name=name,
                width=self.mw, height=self.mh, steps=models.default_steps(name),
                seed=self.seed + i, guidance=None, negative_prompt="",
            )
            (self.dir / f"still_{i:02d}.png").write_bytes(base64.b64decode(result.image_base64))

    def animate(self, seconds: list[float]) -> list[Path | None]:
        clips: list[Path | None] = []
        for i, shot in enumerate(self.shots):
            self._progress(0.35, 0.90, i, "motion", f"Animating shot {i + 1} of {len(self.shots)}")
            clip = self.dir / f"clip_{i:02d}.mp4"
            prompt = f"{self._visual(shot)}. Natural, subtle motion; steady cinematic camera."
            cmd = [
                config.VIDEO_LTX_BIN, "generate", "--prompt", prompt, "--output", str(clip),
                "--model", config.VIDEO_LTX_MODEL,
                "--image", str(self.dir / f"still_{i:02d}.png"), "0", "1.0",
                "--height", str(self.mh), "--width", str(self.mw),
                "--frames", str(clip_frames(shot_seconds(seconds[i]))), "--frame-rate", str(FPS),
                "--seed", str(self.seed + i), "--no-audio", "--quiet", *config.VIDEO_LTX_ARGS,
            ]
            try:
                with generator.exclusive_gpu():
                    _run(cmd, timeout=config.VIDEO_CLIP_TIMEOUT)
                if not clip.exists() or clip.stat().st_size == 0:
                    raise RenderError("no clip was written")
                clips.append(clip)
            except RenderError as exc:
                logger.warning("video %s: shot %d motion failed, using a still: %s", self.job["id"], i + 1, exc)
                self.notes.append(f"shot {i + 1}: animation failed, used a still ({str(exc)[:160]})")
                clips.append(None)
        return clips

    def assemble(self, seconds: list[float], clips: list[Path | None]) -> Path:
        start = 0.90 if self.motion else 0.80
        parts = []
        for i, shot in enumerate(self.shots):
            self._progress(start, 1.0, i, "assembly", f"Editing shot {i + 1} of {len(self.shots)}")
            length = shot_seconds(seconds[i])
            frames = math.ceil(length * FPS)
            part = self.dir / f"shot_{i:02d}.mp4"

            if clips[i] is not None:
                # Freeze the clip's last frame for any narration it does not
                # cover; -t below trims whatever is left over.
                inputs = ["-i", clips[i].name]
                video = (f"[0:v]fps={FPS},scale={self.ow}:{self.oh}:flags=lanczos,setsar=1,"
                         f"tpad=stop_mode=clone:stop_duration={length:.2f}")
            else:
                inputs = ["-i", f"still_{i:02d}.png"]
                video = f"[0:v]{ken_burns(i, frames, self.ow, self.oh)},setsar=1"
            video += f",format=yuv420p,fade=t=in:st=0:d={FADE},fade=t=out:st={length - FADE:.2f}:d={FADE}"

            if self.captions:
                cues = caption_cues(shot["narration"], AUDIO_LEAD, seconds[i], max_words=5 if self.portrait else 7)
                (self.dir / f"shot_{i:02d}.srt").write_text(srt(cues))
                # Relative path, because the subtitles filter has its own
                # escaping rules for ':' and quotes; ffmpeg runs in the job dir.
                video += f",subtitles=shot_{i:02d}.srt:force_style='{caption_style(self.portrait)}'"
            video += "[v]"

            delay = int(AUDIO_LEAD * 1000)
            audio = (f"[1:a]aresample=48000,adelay={delay}:all=1,apad,"
                     f"afade=t=out:st={length - FADE:.2f}:d={FADE}[a]")
            _run([
                config.VIDEO_FFMPEG, "-y", *inputs, "-i", f"voice_{i:02d}.wav",
                "-filter_complex", f"{video};{audio}", "-map", "[v]", "-map", "[a]",
                "-t", f"{length:.2f}", "-r", str(FPS),
                "-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-pix_fmt", "yuv420p",
                "-c:a", "aac", "-b:a", "160k", "-ac", "2",
                part.name,
            ], timeout=900, cwd=self.dir)
            parts.append(part)

        (self.dir / "parts.txt").write_text("".join(f"file '{p.name}'\n" for p in parts))
        final = self.dir / "video.mp4"
        # Every part was encoded with identical settings, so they join without
        # a second encode.
        _run([config.VIDEO_FFMPEG, "-y", "-f", "concat", "-safe", "0", "-i", "parts.txt",
              "-c", "copy", "-movflags", "+faststart", final.name], timeout=600, cwd=self.dir)
        return final


class VideoRenderer:
    """The job queue: persistence, the single render worker, and cleanup."""

    def __init__(self, root: str | Path) -> None:
        self.root = Path(root)
        self._jobs: dict[str, dict] = {}
        self._lock = threading.Lock()
        self._queue: queue.Queue[str] = queue.Queue()
        self._thread: threading.Thread | None = None

    def start(self) -> None:
        if self._thread is not None:
            return
        self._load()
        self._thread = threading.Thread(target=self._work, name="video-render", daemon=True)
        self._thread.start()

    def _load(self) -> None:
        """Read jobs back from disk; requeue the ones a restart interrupted."""
        self.root.mkdir(parents=True, exist_ok=True)
        for path in sorted(self.root.glob("*/job.json"), key=lambda p: p.stat().st_mtime):
            try:
                job = json.loads(path.read_text())
            except (OSError, ValueError):
                continue
            if job.get("status") in (QUEUED, RUNNING):
                if job.get("attempts", 0) >= config.VIDEO_MAX_ATTEMPTS:
                    job.update(status=FAILED, stage="failed", finished_at=time.time(),
                               error="interrupted by a service restart too many times")
                else:
                    job.update(status=QUEUED, stage="queued", message="requeued after a service restart")
                    self._queue.put(job["id"])
                self._save(job)
            self._jobs[job["id"]] = job

    def _save(self, job: dict) -> None:
        workdir = self.root / job["id"]
        workdir.mkdir(parents=True, exist_ok=True)
        tmp = workdir / "job.json.tmp"
        tmp.write_text(json.dumps(job))
        tmp.replace(workdir / "job.json")

    def _update(self, job: dict, **fields) -> None:
        with self._lock:
            job.update(fields)
            self._save(job)

    def submit(self, request: dict) -> tuple[dict, bool]:
        """Queue a render. A repeated id returns the existing job (created=False)."""
        self.prune()
        job_id = request.get("id") or str(uuid.uuid4())
        with self._lock:
            if (existing := self._jobs.get(job_id)) is not None:
                return existing, False
            waiting = sum(1 for j in self._jobs.values() if j["status"] in (QUEUED, RUNNING))
            if waiting > config.VIDEO_MAX_QUEUE:
                raise QueueFull(f"{waiting} videos are already queued or rendering — try again later")
            job = {
                "id": job_id, "status": QUEUED, "stage": "queued", "progress": 0.0,
                "message": "waiting for the GPU", "error": "", "notes": [],
                "created_at": time.time(), "started_at": None, "finished_at": None,
                "attempts": 0, "duration_seconds": 0.0, "shots": len(request["shots"]),
                "title": request.get("title", ""),
                "motion_model": "", "ref": request.get("ref") or {}, "request": request,
            }
            self._jobs[job_id] = job
            self._save(job)
        self._queue.put(job_id)
        return job, True

    def get(self, job_id: str) -> dict | None:
        return self._jobs.get(job_id)

    def list(self) -> list[dict]:
        return sorted(self._jobs.values(), key=lambda j: j["created_at"], reverse=True)

    def file(self, job_id: str) -> Path | None:
        job = self._jobs.get(job_id)
        if job is None or job["status"] != DONE:
            return None
        path = self.root / job_id / "video.mp4"
        return path if path.exists() else None

    def prune(self) -> None:
        cutoff = time.time() - config.VIDEO_KEEP_DAYS * 86400
        with self._lock:
            stale = [j for j in self._jobs.values()
                     if j["status"] in (DONE, FAILED) and (j.get("finished_at") or 0) < cutoff]
            for job in stale:
                del self._jobs[job["id"]]
        for job in stale:
            shutil.rmtree(self.root / job["id"], ignore_errors=True)

    def _work(self) -> None:
        while True:
            job_id = self._queue.get()
            job = self._jobs.get(job_id)
            if job is None or job["status"] != QUEUED:
                continue
            self._update(job, status=RUNNING, stage="starting", started_at=time.time(),
                         attempts=job.get("attempts", 0) + 1, error="")
            render = None
            try:
                render = _Render(job, self.root / job_id, lambda j=job, **kw: self._update(j, **kw))
                final = render.run()
                self._update(job, status=DONE, stage="done", progress=1.0, message="ready",
                             finished_at=time.time(), duration_seconds=round(probe_seconds(final), 2),
                             notes=render.notes,
                             motion_model=config.VIDEO_LTX_MODEL if render.motion else "")
                logger.info("video %s done in %.0fs", job_id, time.time() - job["started_at"])
            except Exception as exc:  # noqa: BLE001 — every failure ends the job, never the worker
                logger.exception("video %s failed", job_id)
                self._update(job, status=FAILED, stage="failed", finished_at=time.time(),
                             error=str(exc)[:1000], notes=render.notes if render else [])


def public(job: dict) -> dict:
    """A job as the API returns it: everything but the request it was made from."""
    return {k: v for k, v in job.items() if k != "request"}


renderer = VideoRenderer(config.VIDEO_DIR)
