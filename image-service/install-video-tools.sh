#!/usr/bin/env bash
# Install what video.py needs on the workstation: ffmpeg-full, a narration
# (mlx-audio / Kokoro) virtualenv, an LTX-2 (ltx-2-mlx) virtualenv, and their
# weights. Idempotent — re-run it to repair or update. Restart the image service
# afterwards so it picks the tools up (it checks for them per request, so a
# restart is only needed when paths change).
#
#   ./install-video-tools.sh                         # LTX-2.3 (ungated)
#   VIDEO_LTX_MODEL=dgrauet/ltx-2.5-mlx-q8 ./install-video-tools.sh
#
# LTX-2.5 is gated on Hugging Face: accept its licence on the model page, run
# `hf auth login` on this host, then install with the line above and set the
# same VIDEO_LTX_MODEL in the service's launchd plist.
set -euo pipefail

TOOLS="${VIDEO_TOOLS_DIR:-$HOME/.jobshout/media-tools}"
LTX_REF="${LTX_REF:-9e4a1a5}"   # ltx-2-mlx commit this service was tested against
LTX_MODEL="${VIDEO_LTX_MODEL:-dgrauet/ltx-2.3-mlx-q8}"
# 2.3 packs read their text encoder from a separate repo; 2.5 packs bundle it.
LTX_GEMMA="mlx-community/gemma-3-12b-it-4bit"
TTS_MODEL="${VIDEO_TTS_MODEL:-mlx-community/Kokoro-82M-bf16}"

eval "$(/opt/homebrew/bin/brew shellenv)"
export HOMEBREW_NO_AUTO_UPDATE=1
# ffmpeg-full, not ffmpeg: the core formula dropped libass and freetype, so
# burning in captions fails with "No such filter: 'subtitles'".
brew list ffmpeg-full >/dev/null 2>&1 || brew install ffmpeg-full
command -v uv >/dev/null 2>&1 || brew install uv
mkdir -p "$TOOLS"

echo "── narration (mlx-audio + Kokoro)"
[[ -x "$TOOLS/tts/.venv/bin/python" ]] || uv venv --quiet --python 3.12 "$TOOLS/tts/.venv"
uv pip install --quiet --python "$TOOLS/tts/.venv/bin/python" "mlx-audio==0.5.8" "misaki[en]==0.9.4" \
  "en_core_web_sm @ https://github.com/explosion/spacy-models/releases/download/en_core_web_sm-3.8.0/en_core_web_sm-3.8.0-py3-none-any.whl"
# Installed above rather than left to misaki, which installs a missing spaCy
# model on first use with a bare `uv pip install` — into whatever virtualenv
# the current directory implies, which here is none.
export VIRTUAL_ENV="$TOOLS/tts/.venv"
# One throwaway line per offered voice (video.VOICES) downloads that voice and
# its language's text front end now. Kokoro fetches each voice on first use,
# and renders run offline, so a voice not warmed here fails mid-render.
warm="$(mktemp -d)"
for voice in af_heart af_bella am_michael am_fenrir bf_emma bm_george hf_alpha hm_omega; do
  case "$voice" in h*) line="तैयार।" ;; *) line="Ready." ;; esac
  "$TOOLS/tts/.venv/bin/mlx_audio.tts.generate" --model "$TTS_MODEL" --voice "$voice" --lang_code "${voice:0:1}" \
    --text "$line" --output_path "$warm" --file_prefix "$voice" --audio_format wav >/dev/null
done
rm -rf "$warm"

echo "── motion (ltx-2-mlx @ $LTX_REF)"
[[ -d "$TOOLS/ltx-2-mlx/.git" ]] || git clone --quiet https://github.com/dgrauet/ltx-2-mlx.git "$TOOLS/ltx-2-mlx"
git -C "$TOOLS/ltx-2-mlx" fetch --quiet origin
git -C "$TOOLS/ltx-2-mlx" checkout --quiet "$LTX_REF"
(cd "$TOOLS/ltx-2-mlx" && uv sync --quiet --all-extras)

echo "── weights: $LTX_MODEL"
HF="$TOOLS/ltx-2-mlx/.venv/bin/hf"
# Only what the distilled pipeline (VIDEO_LTX_ARGS=--distilled) loads. The repos
# also carry a dev transformer and LoRAs for the two-stage pipeline: ~57 GB
# this service never reads. 2.3 ships a newer distilled-1.1 that ltx-2-mlx
# prefers, so its unversioned copy is skipped too; 2.5 has only the one.
exclude=(--exclude "transformer-dev*" --exclude "*distilled-lora*" --exclude "spatial_upscaler_x1_5*")
case "$LTX_MODEL" in
  *ltx-2.3*) exclude+=(--exclude "transformer-distilled.safetensors") ;;
esac
if ! "$HF" download "$LTX_MODEL" "${exclude[@]}" >/dev/null; then
  echo "Could not download $LTX_MODEL. If it is gated (LTX-2.5), accept its licence on" >&2
  echo "huggingface.co and run: $HF auth login" >&2
  exit 1
fi
case "$LTX_MODEL" in
  *ltx-2.3*) "$HF" download "$LTX_GEMMA" >/dev/null ;;
esac

echo "Installed under $TOOLS"
echo "  narration: $TOOLS/tts/.venv/bin/mlx_audio.tts.generate"
echo "  motion:    $TOOLS/ltx-2-mlx/.venv/bin/ltx-2-mlx ($LTX_MODEL)"
