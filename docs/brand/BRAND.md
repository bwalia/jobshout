# JobShout brand

**JobShout is mission control for AI teams.** You say what needs doing; a team
of agents plans it, runs it and reports back.

Open [`index.html`](index.html) for the visual sheet.

## Name

- Write **JobShout**: one word, capital J and S. Not "Jobshout", "Job Shout"
  or "JOBSHOUT".
- Product surfaces: **JobShout.app** (Mac, iPhone, Watch) and **JobShout.com**
  (jobs, showcase, insights). Use the suffix only when the surface matters.

## Taglines

- Primary: **Say it. Your agents ship it.**
- Alternatives: *Shout it. Ship it.* · *Mission control for AI teams.*

## Logo: the Shout wheel

A hub (you) broadcasting to a team of agents. Four long spokes reach the
orbit; four short ones sit inside it, so the wheel has a beat instead of eight
identical sticks. The top-right agent is **live**: amber, with a halo.

| File | Use |
|---|---|
| `logo-icon.svg` | Mac app icon: 824 px rounded-square body on a 1024 canvas, rim and shadow baked in. |
| `logo-icon-small.svg` | Mac app icon at 16–32 px: four spokes, the hub and the live agent, no orbit. |
| `logo-icon-ios.svg` | iPhone, iPad and Watch icon: full-bleed square, the system applies the mask. |
| `logo-glyph.svg` | The wheel without a field: in-app, web headers, social avatars. |
| `logo-glyph-small.svg` | The simplified wheel: favicons and anything under 32 px. |
| `logo-mono.svg` | One colour (`currentColor`): embossing, watermarks, single-colour print. |
| `lockup.svg` / `lockup-on-dark.svg` | Wheel + wordmark, the default logo. |
| `wordmark.svg` / `wordmark-on-dark.svg` | Wordmark alone, where the wheel is already shown nearby. |
| `icon-layers/` | Field and wheel as separate layers, for Icon Composer (Liquid Glass). |
| `png/` | PNG renders (`@2x`, and the 1024 icon masters). |

- **Clear space:** keep free space around the logo equal to the diameter of a
  long-spoke agent node.
- **Minimum size:** the full wheel is 32 px; below that use `logo-glyph-small`.
  The lockup is at least 120 px wide.
- **Don't:** recolour the gradient, rotate the wheel (the live agent always
  sits top right), add or remove spokes, add outlines or effects, or put the
  full-colour wheel on busy photos. Use the mono mark there.
- The wordmark is **Unbounded Bold** converted to outlines (SIL Open Font
  License, `fonts/OFL.txt`). "Job" is Ink on light and Cream on dark; "Shout"
  always carries the Shout gradient.

## Colour

| Token | Hex | Role |
|---|---|---|
| Fuchsia | `#E0218A` | Gradient start |
| Rose | `#FF3D6E` | Gradient middle, the mark's core colour |
| Amber | `#FF9A3C` | Gradient end, warm highlights |
| Live | `#FFC23D` | Live only: the running agent, "running" states |
| Raspberry | `#CC3A63` | UI primary in the web app (every primary action) |
| Plum | `#1A0F18` | App icon field, dark hero sections |
| Ink | `#1C1714` | Text and wordmark on light |
| Cream | `#FFF7EB` | Cards, and text on colour or dark |
| Beige | `#F9F0E0` | Page ground in the web app |
| Sage | `#A2AB73` | Success / done |

**Shout gradient:** 135°, Fuchsia → Rose (50%) → Amber. Use it for the mark,
the "Shout" in the wordmark and hero moments. Never behind body text, and
never for body text itself: it fails contrast.

**Amber means live**, and nothing else, so a running agent is always
recognisable at a glance.

The web app's UI palette (beige, cream, sage, raspberry, warm charcoal) is
defined in `web/nextjs/styles/globals.css`. Raspberry sits in the same hue family
as Rose but is deep enough for cream text on it to pass AA (4.58:1), so buttons
and the mark agree.

## Typography

- **Display:** Unbounded (700 for headlines and the wordmark, 500–600 for subheads).
- **UI and body:** Poppins (400 body, 600 emphasis).
- **Logs, run output, code:** JetBrains Mono.
- **On Apple platforms:** SF Pro for UI. The wordmark stays Unbounded, as artwork.

## Shape and motion

- Round things: nodes, pills, generous corners (about 18% of a shape's height).
- The **hub-and-spoke** is the signature motif: use it for empty states,
  diagrams of a team, and loading states (agents lighting up in turn).
- Motion is quick and calm: 150–250 ms, ease-out. Only the live agent pulses,
  and only while something is actually running.

## Voice

Plain, confident, warm. Short sentences. Say what happened and what's next.

| Do | Don't |
|---|---|
| "Research done. 12 sources, 3 worth reading." | "Your amazing research has been successfully completed!" |
| "The writer agent is waiting on your approval." | "Error: approval_required (code 412)." |
| "Drafted the article. Want it shorter?" | "As an AI, I have generated content for you." |

## Rebuilding

Everything above is generated from one definition of the wheel:

```sh
python3 -m pip install fonttools pillow   # once
brew install librsvg                       # once, for rsvg-convert
python3 docs/brand/tools/build-brand.py
```

It rewrites the SVGs and PNGs here, the Apple asset catalogs
(`ios/{App,Mac/App,Watch/App}/Assets.xcassets/AppIcon.appiconset`) and the
icons, favicons and `public/brand/` files of both web apps.
