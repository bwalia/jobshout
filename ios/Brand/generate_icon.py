#!/usr/bin/env python3
"""Regenerate JobShout app icon masters and wordmark lockups."""

from __future__ import annotations

import math
from pathlib import Path

from PIL import Image, ImageDraw, ImageEnhance, ImageFilter, ImageFont

OUT = Path(__file__).resolve().parent

ROSE = (255, 43, 107)
ROSE_LT = (255, 92, 145)
INK0 = (10, 12, 16)
INK1 = (28, 32, 40)


def lerp(a: tuple[int, int, int], b: tuple[int, int, int], t: float) -> tuple[int, int, int]:
    return tuple(int(a[i] + (b[i] - a[i]) * t) for i in range(3))


def radial_bg(size: int) -> Image.Image:
    img = Image.new("RGB", (size, size), INK0)
    px = img.load()
    cx = cy = size / 2
    maxr = math.hypot(cx, cy)
    for y in range(size):
        for x in range(size):
            t = (math.hypot(x - cx, y - cy) / maxr) ** 1.15
            px[x, y] = lerp(INK1, INK0, t)
    return img


def draw_burst(layer_size: int, color: tuple[int, int, int], scale: float = 1.0) -> Image.Image:
    img = Image.new("RGBA", (layer_size, layer_size), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)
    cx = cy = layer_size / 2
    outer = layer_size * 0.38 * scale
    hub = outer * 0.22
    node = outer * 0.145
    spoke_inner = hub * 1.05
    spoke_outer = outer - node * 1.1
    w0 = layer_size * 0.022 * scale
    w1 = layer_size * 0.011 * scale
    for i in range(8):
        ang = -math.pi / 2 + i * (2 * math.pi / 8)
        dx, dy = math.cos(ang), math.sin(ang)
        px, py = -dy, dx
        x0, y0 = cx + dx * spoke_inner, cy + dy * spoke_inner
        x1, y1 = cx + dx * spoke_outer, cy + dy * spoke_outer
        d.polygon(
            [
                (x0 + px * w0, y0 + py * w0),
                (x0 - px * w0, y0 - py * w0),
                (x1 - px * w1, y1 - py * w1),
                (x1 + px * w1, y1 + py * w1),
            ],
            fill=color + (255,),
        )
        nx = cx + dx * (outer - node * 0.2)
        ny = cy + dy * (outer - node * 0.2)
        d.ellipse([nx - node, ny - node, nx + node, ny + node], fill=color + (255,))
    ring_w = max(2, int(layer_size * 0.008))
    r = outer + node * 0.55
    d.ellipse([cx - r, cy - r, cx + r, cy + r], outline=color + (70,), width=ring_w)
    d.ellipse([cx - hub, cy - hub, cx + hub, cy + hub], fill=color + (255,))
    return img


def make_icon(size: int = 1024) -> Image.Image:
    bg = radial_bg(size)
    glow = draw_burst(size, ROSE, scale=1.02).filter(ImageFilter.GaussianBlur(radius=size * 0.035))
    glow = ImageEnhance.Brightness(glow).enhance(0.55)
    base = Image.alpha_composite(bg.convert("RGBA"), glow)
    base = Image.alpha_composite(base, draw_burst(size, ROSE, scale=1.0))
    return base.convert("RGB")


def squircle_mask(size: int) -> Image.Image:
    m = Image.new("L", (size, size), 0)
    ImageDraw.Draw(m).rounded_rectangle([0, 0, size - 1, size - 1], radius=int(size * 0.223), fill=255)
    return m


def wordmark(dark: bool) -> Image.Image:
    bg = INK0 if dark else (255, 255, 255)
    canvas = Image.new("RGB", (1600, 480), bg)
    tile = Image.new("RGBA", (320, 320), (0, 0, 0, 0))
    ImageDraw.Draw(tile).rounded_rectangle([0, 0, 319, 319], radius=72, fill=INK0 + (255,))
    small = make_icon(280).resize((280, 280), Image.Resampling.LANCZOS).convert("RGBA")
    sm = Image.new("L", (280, 280), 0)
    ImageDraw.Draw(sm).rounded_rectangle([0, 0, 279, 279], radius=56, fill=255)
    small.putalpha(sm)
    tile.paste(small, (20, 20), small)
    canvas.paste(tile, (40, 80), tile)
    try:
        font = ImageFont.truetype("/System/Library/Fonts/SFNS.ttf", 140)
    except OSError:
        try:
            font = ImageFont.truetype("/System/Library/Fonts/Helvetica.ttc", 140)
        except OSError:
            font = ImageFont.load_default()
    ImageDraw.Draw(canvas).text(
        (420, 150), "JobShout", font=font, fill=(255, 255, 255) if dark else (17, 17, 17)
    )
    return canvas


def install_catalogs(master: Path) -> None:
    root = OUT.parent
    for rel in (
        "App/Assets.xcassets/AppIcon.appiconset/AppIcon.png",
        "Mac/App/Assets.xcassets/AppIcon.appiconset/AppIcon.png",
        "Watch/App/Assets.xcassets/AppIcon.appiconset/AppIcon.png",
    ):
        dest = root / rel
        dest.write_bytes(master.read_bytes())


def main() -> None:
    icon = make_icon(1024)
    master = OUT / "AppIcon-1024.png"
    icon.save(master, "PNG", optimize=True)

    preview = icon.convert("RGBA")
    preview.putalpha(squircle_mask(1024))
    preview.save(OUT / "DockPreview-squircle.png", "PNG")

    draw_burst(1024, ROSE).save(OUT / "Mark-rose-transparent.png", "PNG")
    wordmark(False).save(OUT / "Wordmark-light.png", "PNG")
    wordmark(True).save(OUT / "Wordmark-dark.png", "PNG")
    install_catalogs(master)
    print(f"wrote {master} and installed into App/Mac/Watch catalogs")


if __name__ == "__main__":
    main()
