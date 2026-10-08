# JobShout brand

Evolved from the original magenta “wheel”: **eight** spokes (not sixteen), thicker tapered rays, soft broadcast ring, dark field for Dock contrast.

## Mark

**Broadcast burst** — hub + tapered spokes + terminal nodes. Reads as network pulse / shout from a centre.

## Palette

| Token | Hex | Use |
|---|---|---|
| Ink | `#0A0C10` → `#1C2028` | Icon field (radial) |
| Rose | `#FF2B6B` | Primary mark & UI accent |
| Rose soft | `#FF5C91` | Dark-mode accent / highlight |
| Near black | `#111111` | Wordmark on light |
| Paper | `#FFFFFF` | Light surfaces |

## Wordmark

**JobShout** (capital J and S). On Apple platforms prefer SF Pro Display Bold.

## Files

| File | Purpose |
|---|---|
| `AppIcon-1024.png` | App Store / asset-catalog master (full-bleed square, no mask) |
| `Mark.svg` | Vector source |
| `Mark-rose-transparent.png` | Overlays |
| `DockPreview-squircle.png` | Docs / marketing only (Apple applies the real mask) |
| `Wordmark-light.png` / `Wordmark-dark.png` | Lockups |

Regenerate masters:

```sh
python3 ios/Brand/generate_icon.py
```

## App icon rules (Apple)

- Ship **full-bleed** 1024×1024 RGB (no rounded corners, no transparency).
- Keep the mark optically centred with ~12% padding.
- Same master for iPhone, iPad, Mac, Watch asset catalogs.
