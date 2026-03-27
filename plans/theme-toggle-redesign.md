<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Theme Toggle Redesign

Design spec for reshaping, re-animating, and adding clickability affordance to
the website's light/dark mode toggle button.

## Context

The toggle lives in `docs/layouts/partials/theme-toggle.html` with styles in
`docs/static/css/variables.css` and logic in `docs/static/js/theme.js`. It
appears on every page (homepage, docs, blog) via partial inclusion.

The sidebar house icon in `docs/layouts/partials/sidebar.html` is the
canonical house shape. The toggle's house silhouette currently has different
proportions — narrower, shallower roof, smaller relative to its viewBox.

## 1. House Proportions

**Goal**: The toggle house is a uniform scale of the sidebar house polygon.

**Sidebar house** (viewBox `0 0 32 32`):
```
polygon points="16,4 2,16 6,16 6,28 26,28 26,16 30,16"
rect x="13" y="19" width="6" height="9"  (door)
```

**Scale factor**: P = 0.7, centered at x=20 in the 40×32 toggle viewBox,
base pinned to y=32.

**Derivation**: Sidebar polygon relative to center-bottom (16, 28), each
point scaled by 0.7 and translated to (20, 32):

| Sidebar point | Relative to (16,28) | × 0.7 + (20,32) | Toggle point |
|---------------|---------------------|------------------|--------------|
| 16, 4         | 0, −24              | 0, −16.8         | 20, 15.2     |
| 2, 16         | −14, −12            | −9.8, −8.4       | 10.2, 23.6   |
| 6, 16         | −10, −12            | −7, −8.4         | 13, 23.6     |
| 6, 28         | −10, 0              | −7, 0            | 13, 32       |
| 26, 28        | 10, 0               | 7, 0             | 27, 32       |
| 26, 16        | 10, −12             | 7, −8.4          | 27, 23.6     |
| 30, 16        | 14, −12             | 9.8, −8.4        | 29.8, 23.6   |

**Door**: x=17.9, y=25.7, width=4.2, height=6.3

**Implementation**: Extract the sidebar house polygon into a shared Hugo
partial (`layouts/partials/house-shape.html`) and use an SVG `transform` to
scale it in the toggle:

```html
{{/* house-shape.html — single source of truth */}}
<polygon points="16,4 2,16 6,16 6,28 26,28 26,16 30,16" fill="{{ .fill }}"/>
<rect x="13" y="19" width="6" height="9" fill="{{ .door }}"/>
```

```html
{{/* In theme-toggle.html — same polygon, scaled via transform */}}
<g transform="translate(20,32) scale(0.7) translate(-16,-28)">
  {{ partial "house-shape.html" (dict "fill" "var(--charcoal-soft)" "door" "var(--toggle-bg)") }}
</g>
```

The transform chain: move origin to sidebar center-bottom (16,28), scale by
0.7, reposition to toggle center-bottom (20,32). This produces the same
coordinates as the derivation table above, but the polygon is defined once.

The door fill uses `--toggle-bg` so it matches the button background and reads
as a cutout.

The eave line sits at y=23.6, which serves as the visual horizon for celestial
body animations.

## 2. Celestial Animation — Arc Paths

**Goal**: Sun and moon rise/set in natural arcs behind the house, rather than
the current scale-in-place / slide-from-above.

### Resting positions

- **Sun** (light mode): upper-right sky, roughly cx=30 cy=8
- **Moon** (dark mode): upper-left sky, roughly cx=10 cy=8

### Transition: light → dark (click)

1. **Sun arcs down** from upper-right, curving toward center, disappearing
   behind the house (below the y=23.6 eave line). Opacity fades as it
   crosses the horizon.
2. **Moon arcs up** from behind the house center, curving left to its
   resting position in the upper-left. Opacity fades in as it clears
   the horizon.
3. **Stars** fade in (opacity 0 → 1) as the sky darkens.
4. **Button background** transitions from light linen to `#342f29`.

### Transition: dark → light (click)

Exact reverse: moon arcs down behind the house, sun arcs up to upper-right.
Stars fade out.

### Timing

- Total duration: ~800ms
- Sun/moon arcs overlap slightly (staggered by ~100–200ms) so neither the
  sky nor the scene is ever empty.
- Easing: `cubic-bezier` with slight overshoot for natural feel on the
  arriving body; ease-in for the departing body.

### Implementation approach

Use CSS `transition` on `transform` for the arc motion. Each celestial body
has two states defined by CSS custom properties or class-based transforms:

- **Resting** (visible): positioned at its sky location, opacity 1
  - Sun: approximately translate(0, 0) at cx=30 cy=8
  - Moon: approximately translate(0, 0) at cx=10 cy=8
- **Hidden** (behind house): translated toward house center below eave line,
  opacity 0
  - Sun hidden: approximately cx=20 cy=30 (behind house, below horizon)
  - Moon hidden: approximately cx=20 cy=30 (same convergence point)

The `data-theme="dark"` attribute toggles which state each body is in.
The arc shape comes from combining `translateX` + `translateY` with different
transition durations/easings on each axis, creating a curved path without
`@keyframes` or `motion-path`.

### prefers-reduced-motion

Instant swap — no arc, no fade. Same as current behavior.

## 3. Button Affordance

**Goal**: Make the toggle visually interactive without adding a border.

### Background fill

- **Light mode**: `var(--linen)` (#f0ebe4) — one step darker than page
- **Dark mode**: `#342f29` — warm mid-tone, clear contrast against page
  (#242019)
- **Hover (light)**: one step deeper, approximately #e8e2da
- **Hover (dark)**: one step lighter, approximately #3d3730 (`var(--rule)`)
- `border-radius: 6px` — matches search input, CTA buttons, and other
  interactive elements in the existing radius tier system
- `transition: background-color 0.15s` for hover
- No border (border: none)
- `cursor: pointer` (already present)

### Dark mode background variable

Add a new CSS variable or use a conditional approach:
```css
.theme-toggle {
  background: var(--linen);
  border-radius: 6px;
}
[data-theme="dark"] .theme-toggle {
  background: #342f29;
}
```

The door cutout `fill` matches the button background so it reads as a
window into the page beneath.

## 4. Files Changed

| File | Change |
|------|--------|
| `docs/layouts/partials/theme-toggle.html` | Replace house path with scaled sidebar polygon; adjust sun/moon positions |
| `docs/static/css/variables.css` | New animation properties, button background styles, hover states |
| `docs/static/js/theme.js` | Update sun-drift logic for arc transitions (may simplify if pure CSS) |

## 5. Preserved Behavior

- `aria-label="Toggle dark mode"` — unchanged
- `localStorage` theme persistence — unchanged
- `sessionStorage` sun-animation-seen tracking — may simplify or remove if
  arcs are CSS-transition-driven (no `animationend` needed)
- `theme-changed` custom event for Mermaid re-rendering — unchanged
- System `prefers-color-scheme` listener — unchanged
- Chimney smoke and house crumble scripts — unaffected (different SVG targets)

## 6. Existing Radius Tier System

For reference, to avoid introducing inconsistencies:

| Radius | Element type |
|--------|-------------|
| 4px | Inline code, small tags, kbd |
| 6px | Interactive — CTA buttons, search input, pagefind, blockquotes |
| 8px | Large content — code blocks, videos |
| 10px | Overlays — search modal |

The toggle uses **6px** (interactive element tier).
