<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Website Dark Mode

## Overview

Add dark mode to the Hugo website (homepage, docs, blog) with a toggle button
in the header navigation. Respects `prefers-color-scheme` by default, with
manual override persisted in localStorage.

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

## Detection Logic

1. Page load: check `localStorage.getItem("theme")`
2. If no stored preference: check `prefers-color-scheme: dark` media query
3. Apply `data-theme="dark"` attribute on `<html>` if either matches
4. Toggle click: flip attribute, save to localStorage
5. Script runs synchronously in `<head>` to prevent flash of wrong theme

## Toggle Button

- Position: header nav bar, right side, after other nav items
- Icon: moon (`☾`) in light mode, sun (`☀`) in dark mode
- Instant swap on click, no animation
- `<button>` element with `aria-label="Toggle dark mode"`

## Files to Modify

### CSS

- **`static/css/variables.css`** — Add `[data-theme="dark"] { ... }` block
  overriding all custom properties
- **`static/css/syntax.css`** — Add dark syntax highlighting variant under
  `[data-theme="dark"]` (Gruvbox dark colors)
- **`static/css/website.css`** — Toggle button styles for homepage header
- **`static/css/docs.css`** — Toggle button styles for docs header
- **`static/css/blog.css`** — Toggle button styles for blog header

### JavaScript

- **`static/js/theme.js`** (new) — Theme detection, toggle logic, localStorage
  persistence (~30 lines)
- **`static/js/house-crumble.js`** — Read colors from CSS custom properties via
  `getComputedStyle` instead of hardcoded hex values
- **`static/js/chimney-smoke.js`** — Same: use CSS custom properties

### Templates

- **`layouts/index.html`** — Add toggle button in header, include `theme.js`
  in `<head>`
- **`layouts/_default/baseof.html`** — Add toggle button in header, include
  `theme.js` in `<head>`, update Mermaid config to respect `data-theme`
- **`layouts/blog/baseof.html`** — Add toggle button in header, include
  `theme.js` in `<head>`

## Out of Scope

- No new CSS files or build tools
- No changes to fonts, layout, or content structure
- No separate dark mode images (current WebP screenshots are fine)
- Pagefind search UI styling (uses its own theme, can be addressed later)
