#!/usr/bin/env python3
# Usage (from the repo root):
#   python3 -m pip install fonttools pillow   # once
#   brew install librsvg                       # once, for rsvg-convert
#   python3 docs/brand/tools/build-brand.py
"""Builds every JobShout brand asset from one definition of the mark.

Writes the SVG sources to docs/brand, renders them to PNG with rsvg-convert,
and copies the results into the Apple asset catalogs and both web apps'
icons. Edit the geometry here, never the generated files.
"""
from __future__ import annotations

import io
import math
import shutil
import subprocess
from pathlib import Path

from fontTools.pens.boundsPen import BoundsPen
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.transformPen import TransformPen
from fontTools.ttLib import TTFont
from fontTools.varLib.instancer import instantiateVariableFont
from PIL import Image

ROOT = Path(__file__).resolve().parents[3]
BRAND = ROOT / "docs/brand"

# ── Palette ──────────────────────────────────────────────────────────────────
FUCHSIA = "#E0218A"
ROSE = "#FF3D6E"
AMBER = "#FF9A3C"
LIVE = "#FFC23D"
RASPBERRY = "#CC3A63"
INK = "#1C1714"
PLUM = "#1A0F18"
CREAM = "#FFF7EB"

# ── The mark: the Shout wheel ────────────────────────────────────────────────
# A hub (you) broadcasting to a team of agents. Four long spokes reach the
# orbit; four short ones sit inside it, so the wheel has a beat instead of
# eight identical sticks. The top-right agent is live. All in a 1024 box
# centred on (512, 512).
C = 512
HUB_R = 112
CORE_R = 40
LONG_R, LONG_NODE = 318, 62
SHORT_R, SHORT_NODE = 238, 44
ORBIT_W = 14
ORBIT_GAP = 24  # clear space between the orbit and a long node
LIVE_ANGLE = -45
HALO_GAP, HALO_W = 16, 10


def polar(r: float, deg: float) -> tuple[float, float]:
    a = math.radians(deg)
    return C + r * math.cos(a), C + r * math.sin(a)


def spoke(deg: float, r_end: float, w0: float, w1: float) -> str:
    """A tapered spoke from inside the hub to a node centre."""
    a = math.radians(deg)
    dx, dy = math.cos(a), math.sin(a)
    px, py = -dy, dx
    r0 = HUB_R - 20
    x0, y0 = C + dx * r0, C + dy * r0
    x1, y1 = C + dx * r_end, C + dy * r_end
    pts = [
        (x0 + px * w0, y0 + py * w0),
        (x1 + px * w1, y1 + py * w1),
        (x1 - px * w1, y1 - py * w1),
        (x0 - px * w0, y0 - py * w0),
    ]
    return "M" + " L".join(f"{x:.1f},{y:.1f}" for x, y in pts) + " Z"


def orbit_arcs() -> str:
    """The orbit, broken where it meets each long node."""
    gap = math.degrees(math.asin((LONG_NODE + ORBIT_GAP) / LONG_R))
    d = []
    for start in (-90, 0, 90, 180):
        a0, a1 = start + gap, start + 90 - gap
        x0, y0 = polar(LONG_R, a0)
        x1, y1 = polar(LONG_R, a1)
        d.append(f"M{x0:.1f},{y0:.1f} A{LONG_R},{LONG_R} 0 0 1 {x1:.1f},{y1:.1f}")
    return " ".join(d)


def glyph(fill: str, *, live: str, small: bool = False, depth: bool = False,
          core: str | None = CREAM, orbit_opacity: float = 0.5) -> str:
    """SVG elements for the mark. `fill` is a colour or url(#id)."""
    parts = []
    if small:
        # 16–32 px: four thick spokes, the hub and the live agent. No orbit.
        for deg in (-90, 0, 90, 180):
            parts.append(f'<path d="{spoke(deg, LONG_R, 30, 24)}"/>')
            x, y = polar(LONG_R, deg)
            parts.append(f'<circle cx="{x:.1f}" cy="{y:.1f}" r="{LONG_NODE + 16}"/>')
        parts.append(f'<path d="{spoke(LIVE_ANGLE, LONG_R * 0.8, 30, 24)}"/>')
        hub_r = HUB_R + 30
    else:
        for deg in (-90, 0, 90, 180):
            parts.append(f'<path d="{spoke(deg, LONG_R, 23, 12)}"/>')
            x, y = polar(LONG_R, deg)
            parts.append(f'<circle cx="{x:.1f}" cy="{y:.1f}" r="{LONG_NODE}"/>')
        for deg in (-135, 45, 135):
            parts.append(f'<path d="{spoke(deg, SHORT_R, 18, 9)}"/>')
            x, y = polar(SHORT_R, deg)
            parts.append(f'<circle cx="{x:.1f}" cy="{y:.1f}" r="{SHORT_NODE}"/>')
        parts.append(f'<path d="{spoke(LIVE_ANGLE, SHORT_R, 18, 9)}"/>')
        hub_r = HUB_R
    parts.append(f'<circle cx="{C}" cy="{C}" r="{hub_r}"/>')
    body = f'<g fill="{fill}">{"".join(parts)}</g>'

    extra = []
    if not small:
        extra.append(
            f'<path d="{orbit_arcs()}" fill="none" stroke="{fill}" stroke-width="{ORBIT_W}" '
            f'stroke-linecap="round" stroke-opacity="{orbit_opacity}"/>'
        )
    if depth:
        extra.append(f'<circle cx="{C}" cy="{C}" r="{hub_r}" fill="url(#sheen)"/>')
    if core:
        extra.append(f'<circle cx="{C}" cy="{C}" r="{CORE_R + (14 if small else 0)}" fill="{core}"/>')
    # The live agent: solid amber with a halo.
    r_live = SHORT_R if not small else LONG_R * 0.8
    n_live = SHORT_NODE if not small else LONG_NODE + 8
    lx, ly = polar(r_live, LIVE_ANGLE)
    if not small:
        extra.append(
            f'<circle cx="{lx:.1f}" cy="{ly:.1f}" r="{n_live + HALO_GAP + HALO_W / 2}" fill="none" '
            f'stroke="{live}" stroke-width="{HALO_W}" stroke-opacity="0.45"/>'
        )
    extra.append(f'<circle cx="{lx:.1f}" cy="{ly:.1f}" r="{n_live}" fill="{live}"/>')
    return body + "".join(extra)


def shout_gradient(gid: str = "shout", x1=150, y1=150, x2=874, y2=874) -> str:
    return (
        f'<linearGradient id="{gid}" gradientUnits="userSpaceOnUse" '
        f'x1="{x1}" y1="{y1}" x2="{x2}" y2="{y2}">'
        f'<stop offset="0" stop-color="{FUCHSIA}"/>'
        f'<stop offset="0.5" stop-color="{ROSE}"/>'
        f'<stop offset="1" stop-color="{AMBER}"/></linearGradient>'
    )


SHEEN = (
    '<linearGradient id="sheen" x1="0" y1="0" x2="0" y2="1">'
    '<stop offset="0" stop-color="#FFFFFF" stop-opacity="0.38"/>'
    '<stop offset="0.5" stop-color="#FFFFFF" stop-opacity="0.04"/>'
    '<stop offset="1" stop-color="#FFFFFF" stop-opacity="0"/></linearGradient>'
)

FIELD = f"""
    <radialGradient id="field" cx="0.5" cy="0.35" r="0.85">
      <stop offset="0" stop-color="#2C1527"/><stop offset="0.3" stop-color="#251222"/><stop offset="0.55" stop-color="#1E101C"/><stop offset="0.8" stop-color="{PLUM}"/><stop offset="1" stop-color="#0E090E"/>
    </radialGradient>
    <radialGradient id="glowA" cx="0.22" cy="0.15" r="0.75">
      <stop offset="0" stop-color="{FUCHSIA}" stop-opacity="0.40"/><stop offset="0.6" stop-color="{FUCHSIA}" stop-opacity="0"/>
    </radialGradient>
    <radialGradient id="glowB" cx="0.88" cy="0.92" r="0.6">
      <stop offset="0" stop-color="{AMBER}" stop-opacity="0.26"/><stop offset="1" stop-color="{AMBER}" stop-opacity="0"/>
    </radialGradient>
    <radialGradient id="hubGlow" cx="0.5" cy="0.5" r="0.42">
      <stop offset="0" stop-color="{ROSE}" stop-opacity="0.5"/><stop offset="0.4" stop-color="{ROSE}" stop-opacity="0.16"/><stop offset="1" stop-color="{ROSE}" stop-opacity="0"/>
    </radialGradient>
    <linearGradient id="rim" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#FFFFFF" stop-opacity="0.30"/><stop offset="0.3" stop-color="#FFFFFF" stop-opacity="0.05"/><stop offset="1" stop-color="#FFFFFF" stop-opacity="0.10"/>
    </linearGradient>
    <filter id="lift" x="-20%" y="-20%" width="140%" height="150%">
      <feDropShadow dx="0" dy="18" stdDeviation="22" flood-color="#050205" flood-opacity="0.6"/>
    </filter>
    <filter id="iconShadow" x="-10%" y="-10%" width="120%" height="125%">
      <feDropShadow dx="0" dy="14" stdDeviation="16" flood-color="#000000" flood-opacity="0.38"/>
    </filter>"""


def scaled(inner: str, s: float) -> str:
    return f'<g transform="translate({C} {C}) scale({s}) translate({-C} {-C})">{inner}</g>'


def field_art(s: float, *, small: bool = False) -> str:
    """Glow, hub bloom and the lifted mark, for the icon field."""
    return (
        '<rect width="1024" height="1024" fill="url(#hubGlow)"/>'
        + f'<g filter="url(#lift)">{scaled(glyph("url(#shout)", live=LIVE, small=small, depth=True), s)}</g>'
    )


def svg(w: float, h: float, title: str, defs: str, body: str, size: float | None = None) -> str:
    dw, dh = (size, size) if size else (w, h)
    return (
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {w:g} {h:g}" width="{dw:g}" height="{dh:g}">\n'
        f"  <title>{title}</title>\n  <defs>{defs}</defs>\n  {body}\n</svg>\n"
    )


def icon_mac(small: bool = False) -> str:
    """macOS grid: an 824 px body on a 1024 canvas, the shape baked in."""
    body = f'<rect x="100" y="100" width="824" height="824" rx="186"'
    art = (
        f'<g filter="url(#iconShadow)">{body} fill="{PLUM}"/></g>{body} fill="url(#field)"/>'
        f'<clipPath id="bodyClip">{body}/></clipPath>'
        f'<g clip-path="url(#bodyClip)"><rect x="100" y="100" width="824" height="824" fill="url(#glowA)"/>'
        f'<rect x="100" y="100" width="824" height="824" fill="url(#glowB)"/>'
        + field_art(0.84 if small else 0.76, small=small)
        + "</g>"
        + f'<rect x="102" y="102" width="820" height="820" rx="184" fill="none" stroke="url(#rim)" stroke-width="4"/>'
    )
    return svg(1024, 1024, "JobShout app icon (macOS)", FIELD + shout_gradient() + SHEEN, art)


def icon_full_bleed() -> str:
    """iPhone, iPad and Watch: full-bleed square, the system applies the mask."""
    art = (
        '<rect width="1024" height="1024" fill="url(#field)"/>'
        '<rect width="1024" height="1024" fill="url(#glowA)"/>'
        '<rect width="1024" height="1024" fill="url(#glowB)"/>'
        + field_art(0.9)
    )
    return svg(1024, 1024, "JobShout app icon (iOS, iPadOS, watchOS)", FIELD + shout_gradient() + SHEEN, art)


# The glyph's own box: long nodes reach 512 ± 380.
G0, GS = 512 - 392, 784


def glyph_svg(small: bool = False) -> str:
    return svg(
        GS, GS, "JobShout mark",
        shout_gradient() + SHEEN,
        f'<g transform="translate({-G0} {-G0})">{glyph("url(#shout)", live=LIVE, small=small)}</g>',
        size=GS // 8,
    )


def mono_svg() -> str:
    # One colour: the halo keeps the live agent distinct, the core is knocked out.
    body = glyph("currentColor", live="currentColor", core=None, orbit_opacity=1)
    return svg(
        GS, GS, "JobShout mark, one colour",
        f'<mask id="core"><rect width="1024" height="1024" fill="#fff"/><circle cx="{C}" cy="{C}" r="{CORE_R}" fill="#000"/></mask>',
        f'<g transform="translate({-G0} {-G0})" mask="url(#core)">{body}</g>',
    )


# ── Wordmark: Unbounded Bold, converted to outlines ──────────────────────────
font = instantiateVariableFont(TTFont(BRAND / "fonts/Unbounded-Variable.ttf"), {"wght": 700})
glyphs = font.getGlyphSet()
cmap = font.getBestCmap()
UPM = font["head"].unitsPerEm
TRACKING = -0.02


def text_path(text: str, size: float, x: float, baseline: float) -> tuple[str, float]:
    s = size / UPM
    pen = SVGPathPen(glyphs)
    cx = 0.0
    for ch in text:
        g = cmap[ord(ch)]
        glyphs[g].draw(TransformPen(pen, (s, 0, 0, -s, x + cx, baseline)))
        cx += glyphs[g].width * s + TRACKING * size
    return pen.getCommands(), cx - TRACKING * size


def cap_height(size: float) -> float:
    bp = BoundsPen(glyphs)
    glyphs[cmap[ord("J")]].draw(bp)
    return bp.bounds[3] * size / UPM


def words(x: float, baseline: float, size: float, job: str) -> tuple[str, float]:
    """'Job' in ink or cream, 'Shout' in the shout gradient."""
    jd, jw = text_path("Job", size, x, baseline)
    sx = x + jw + TRACKING * size
    sd, sw = text_path("Shout", size, sx, baseline)
    grad = shout_gradient("shoutText", sx, baseline - cap_height(size), sx + sw, baseline)
    return (
        f'<defs>{grad}</defs><path fill="{job}" d="{jd}"/><path fill="url(#shoutText)" d="{sd}"/>',
        sx + sw - x,
    )


def lockup(job: str, title: str) -> str:
    h, mark = 300, 220
    gx, gy = 30, (300 - mark) / 2
    size = 128
    tx = gx + mark + 44
    baseline = h / 2 + cap_height(size) / 2
    text, tw = words(tx, baseline, size, job)
    w = tx + tw + 36
    mark_g = f'<g transform="translate({gx} {gy}) scale({mark / GS:.5f}) translate({-G0} {-G0})">{glyph("url(#shout)", live=LIVE)}</g>'
    return svg(round(w), h, title, shout_gradient() + SHEEN, mark_g + text)


def wordmark(job: str, title: str) -> str:
    size = 128
    baseline = 40 + cap_height(size)
    text, tw = words(24, baseline, size, job)
    return svg(round(tw + 48), round(baseline + 44), title, "", text)


# ── Write, render, distribute ────────────────────────────────────────────────
def write(name: str, content: str) -> Path:
    p = BRAND / name
    p.write_text(content)
    return p


def render(src: Path, dst: Path, width: int, height: int | None = None) -> None:
    dst.parent.mkdir(parents=True, exist_ok=True)
    cmd = ["rsvg-convert", "-w", str(width)]
    if height:
        cmd += ["-h", str(height)]
    subprocess.run(cmd + ["-o", str(dst), str(src)], check=True)


def flatten(png: Path) -> None:
    """App Store icons must be opaque RGB."""
    Image.open(png).convert("RGB").save(png)


def main() -> None:
    files = {
        "logo-icon.svg": icon_mac(),
        "logo-icon-small.svg": icon_mac(small=True),
        "logo-icon-ios.svg": icon_full_bleed(),
        "logo-glyph.svg": glyph_svg(),
        "logo-glyph-small.svg": glyph_svg(small=True),
        "logo-mono.svg": mono_svg(),
        "lockup.svg": lockup(INK, "JobShout logo, horizontal lockup, for light backgrounds"),
        "lockup-on-dark.svg": lockup(CREAM, "JobShout logo, horizontal lockup, for dark backgrounds"),
        "wordmark.svg": wordmark(INK, "JobShout wordmark, for light backgrounds"),
        "wordmark-on-dark.svg": wordmark(CREAM, "JobShout wordmark, for dark backgrounds"),
    }
    paths = {k: write(k, v) for k, v in files.items()}

    # Icon Composer layers (macOS 26 / iOS 26 Liquid Glass).
    layers = BRAND / "icon-layers"
    layers.mkdir(exist_ok=True)
    (layers / "1-field.svg").write_text(svg(
        1024, 1024, "JobShout icon layer 1: field", FIELD,
        '<rect width="1024" height="1024" fill="url(#field)"/><rect width="1024" height="1024" fill="url(#glowA)"/>'
        '<rect width="1024" height="1024" fill="url(#glowB)"/>'))
    (layers / "2-wheel.svg").write_text(svg(
        1024, 1024, "JobShout icon layer 2: the wheel", shout_gradient() + SHEEN,
        scaled(glyph("url(#shout)", live=LIVE, depth=True), 0.9)))

    png = BRAND / "png"
    render(paths["logo-icon.svg"], png / "icon-mac-1024.png", 1024)
    render(paths["logo-icon-small.svg"], png / "icon-mac-small-1024.png", 1024)
    render(paths["logo-icon-ios.svg"], png / "icon-ios-1024.png", 1024)
    flatten(png / "icon-ios-1024.png")
    render(paths["logo-glyph.svg"], png / "glyph@2x.png", 512)
    for name in ("lockup", "lockup-on-dark", "wordmark", "wordmark-on-dark"):
        src = paths[f"{name}.svg"]
        w = int(src.read_text().split('width="', 2)[1].split('"')[0])
        render(src, png / f"{name}@2x.png", w * 2)

    # Apple asset catalogs.
    ios = ROOT / "ios"
    for cat in ("App", "Watch/App"):
        shutil.copy(png / "icon-ios-1024.png", ios / cat / "Assets.xcassets/AppIcon.appiconset/AppIcon.png")
    mac_set = ios / "Mac/App/Assets.xcassets/AppIcon.appiconset"
    shutil.copy(png / "icon-mac-1024.png", mac_set / "AppIcon.png")
    render(paths["logo-icon-small.svg"], mac_set / "AppIcon-small.png", 64)
    shutil.copy(png / "icon-ios-1024.png", ios / "Brand/AppIcon-1024.png")
    shutil.copy(png / "icon-mac-1024.png", ios / "Brand/AppIcon-mac-1024.png")

    # Web: SVG icon, Apple touch icon and a real favicon.ico, for both apps.
    for app in (ROOT / "web/nextjs", ROOT / "jobshout-com/web/nextjs"):
        brand_dir = app / "public/brand"
        brand_dir.mkdir(parents=True, exist_ok=True)
        for name in ("logo-glyph.svg", "logo-glyph-small.svg", "logo-mono.svg", "lockup.svg", "lockup-on-dark.svg"):
            shutil.copy(paths[name], brand_dir / name)
        shutil.copy(paths["logo-glyph-small.svg"], app / "app/icon.svg")
        render(paths["logo-icon-ios.svg"], app / "app/apple-icon.png", 180)
        flatten(app / "app/apple-icon.png")
        sizes = []
        for px in (16, 32, 48):
            buf = io.BytesIO(subprocess.run(
                ["rsvg-convert", "-w", str(px), str(paths["logo-glyph-small.svg"])],
                check=True, capture_output=True).stdout)
            sizes.append(Image.open(buf).convert("RGBA"))
        sizes[-1].save(app / "app/favicon.ico", sizes=[(s.width, s.height) for s in sizes], append_images=sizes[:-1])

    print("Built", len(files), "SVGs and their renders.")


if __name__ == "__main__":
    main()
