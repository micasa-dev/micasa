<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Website Dark Mode

## Overview

Add dark mode to the Hugo website (homepage, docs, blog) with a toggle button
fixed to the bottom-right corner. Respects `prefers-color-scheme` by default,
with manual override persisted in localStorage. Listens for OS-level theme
changes at runtime when no override exists.

## Palette

Warm dark variant that preserves the earthy brand identity. Base: `#242019`.

| Variable           | Light     | Dark      | Notes                      |
|--------------------|-----------|-----------|----------------------------|
| `--cream`          | `#faf6f1` | `#242019` | Page background            |
| `--linen`          | `#f0ebe4` | `#1c1916` | Surface/code blocks        |
| `--charcoal`       | `#2d2a26` | `#e8ddd0` | Primary text               |
| `--charcoal-soft`  | `#4a4640` | `#a89e91` | Secondary text             |
| `--terracotta`     | `#c05e3c` | `#d4764e` | Primary accent (lightened)  |
| `--terracotta-dark`| `#a04e30` | `#e0835a` | Accent hover               |
| `--sage`           | `#6b8f71` | `#7fa882` | Secondary accent (lightened)|
| `--warm-gray`      | `#9e958a` | `#706760` | Muted/disabled             |
| `--rule`           | `#d9d2c9` | `#3d3730` | Borders/dividers           |
| `--code-bg`        | `#2d2a26` | `#1c1916` | Code block background      |
| `--code-text`      | `#e8e2da` | `#e8ddd0` | Code block text            |
| `--shadow-color`   | `rgba(45,42,38,0.12)` | `rgba(0,0,0,0.3)` | Box shadows |
| `--overlay-bg`     | `rgba(45,42,38,0.5)`  | `rgba(0,0,0,0.5)` | Search overlay |

## Detection Logic

1. Page load: check `localStorage.getItem("theme")`
2. If no stored preference: check `prefers-color-scheme: dark` media query
3. Apply `data-theme="dark"` attribute on `<html>` if either matches
4. Toggle click: flip attribute, save to localStorage
5. Script runs synchronously in `<head>` to prevent flash of wrong theme
6. Runtime `prefers-color-scheme` change listener updates theme when no
   localStorage override exists

## Toggle Button

- Position: fixed bottom-right corner of the page
- SVG house scene with animated sun/moon:
  - Light mode: terracotta sun glow (concentric circles) behind a house
    silhouette with an evenodd door cutout so the glow shines through.
    Sun rises and shrinks to the moon sky position over 1 second (one-shot).
  - Dark mode: crescent moon and 5 twinkling stars in the sky above the house.
    Moon slides in, stars fade in with staggered twinkle animation.
- House fill uses `var(--charcoal-soft)` for a darker silhouette in light mode.
- `<button>` element with `aria-label="Toggle dark mode"`
- All animations respect `prefers-reduced-motion`

## Files Changed

### CSS

- **`static/css/variables.css`** — Dark palette overrides under
  `[data-theme="dark"]`, plus all shared toggle CSS (button positioning,
  sun/moon/star animations, keyframes, reduced-motion overrides)
- **`static/css/website.css`** — `pre` blocks use `--code-bg`/`--code-text`,
  shadows use `--shadow-color`
- **`static/css/docs.css`** — Same variable substitutions, search overlay
  uses `--overlay-bg`
- **`static/css/blog.css`** — Same variable substitutions
- **`static/css/syntax.css`** — Unchanged (Gruvbox colors work on dark
  `--code-bg` in both modes)

### JavaScript

- **`static/js/theme.js`** (new, ~30 lines) — Theme detection, toggle,
  localStorage persistence, OS-level change listener
- **`static/js/house-crumble.js`** — Rubble smoke reads `--warm-gray` from
  CSS instead of hardcoded hex

### Templates

- **`layouts/partials/theme-toggle.html`** (new) — SVG toggle button
  extracted into a Hugo partial, included by all three templates
- **`layouts/index.html`** — Include `theme.js` in `<head>`, include
  theme-toggle partial before `</body>`
- **`layouts/_default/baseof.html`** — Include `theme.js` in `<head>`,
  include theme-toggle partial before `</body>`, update Mermaid config
  to re-render on theme switch with `data-original-source` preservation
- **`layouts/blog/baseof.html`** — Include `theme.js` in `<head>`, include
  theme-toggle partial before `</body>`
- **`layouts/_default/_markup/render-codeblock-mermaid.html`** — Add
  `data-original-source` attribute for theme-switch re-rendering

## What Did Not Change

- No new CSS files or build tools
- No changes to fonts, layout, or content structure
- No separate dark mode images
- `chimney-smoke.js` already uses CSS variables via `.smoke-particle` class
- `syntax.css` Gruvbox colors work as-is on dark code-bg
