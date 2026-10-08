# JobShout brand (Apple)

The brand lives in [`docs/brand/`](../../docs/brand/BRAND.md): the Shout wheel,
palette, type and voice. This folder holds the Apple masters it generates.

| File | Purpose |
|---|---|
| `AppIcon-1024.png` | iPhone, iPad and Watch master: full-bleed, opaque, no corners (the system masks it) |
| `AppIcon-mac-1024.png` | Mac master: 824 px rounded-square body with rim and shadow on a 1024 canvas |
| `Lockup-light.png` / `Lockup-dark.png` | Wheel + wordmark, for App Store and marketing artwork |
| `APP_STORE.md` | Mac App Store packaging notes |

The Mac asset catalog also carries `AppIcon-small.png` (four spokes, no orbit)
for the 16 and 32 px slots, where the full wheel turns to mush.

Regenerate everything, including these files and the asset catalogs:

```sh
python3 -m pip install fonttools pillow && brew install librsvg   # once
python3 docs/brand/tools/build-brand.py
```

For Liquid Glass (macOS 26 / iOS 26), build an `AppIcon.icon` in Icon Composer
from `docs/brand/icon-layers/`.
