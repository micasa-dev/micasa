<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Theme Toggle Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reshape the theme toggle house to match sidebar proportions, add arc-based celestial animations, and add a background-fill clickability affordance.

**Architecture:** Pure CSS/SVG/HTML change across 4 files. The house polygon is extracted into a shared Hugo partial used by both the sidebar and toggle (DRY). The toggle applies an SVG `transform` to scale by P=0.7. Sun/moon use split-axis CSS transitions (nested SVG `<g>` wrappers) to create curved arc paths. The button gets a `--toggle-bg` background with hover states.

**Tech Stack:** SVG, CSS transitions, Hugo partials, vanilla JS (minimal changes)

**Design spec:** `plans/theme-toggle-redesign.md`

---

### Task 1: Add Button Affordance CSS

**Files:**
- Modify: `docs/static/css/variables.css:6-21` (add `--toggle-bg` variable)
- Modify: `docs/static/css/variables.css:41-57` (update `.theme-toggle` and hover)

- [ ] **Step 1: Add `--toggle-bg` CSS variable to both themes**

In `docs/static/css/variables.css`, add `--toggle-bg` and `--toggle-bg-hover`
to `:root` and the dark theme block. Also remove `--sun-final` (no longer
needed after the arc animation replaces sun-drift):

```css
:root {
  --cream: #faf6f1;
  --linen: #f0ebe4;
  --charcoal: #2d2a26;
  --charcoal-soft: #4a4640;
  --terracotta: #c05e3c;
  --terracotta-dark: #a04e30;
  --sage: #6b8f71;
  --warm-gray: #9e958a;
  --rule: #d9d2c9;
  --code-bg: #2d2a26;
  --code-text: #e8e2da;
  --shadow-color: rgba(45, 42, 38, 0.12);
  --overlay-bg: rgba(45, 42, 38, 0.5);
  --toggle-bg: var(--linen);
  --toggle-bg-hover: #e8e2da;
}

[data-theme="dark"] {
  --cream: #242019;
  --linen: #1c1916;
  --charcoal: #e8ddd0;
  --charcoal-soft: #a89e91;
  --terracotta: #d4764e;
  --terracotta-dark: #e0835a;
  --sage: #7fa882;
  --warm-gray: #706760;
  --rule: #3d3730;
  --code-bg: #1c1916;
  --code-text: #e8ddd0;
  --shadow-color: rgba(0, 0, 0, 0.3);
  --overlay-bg: rgba(0, 0, 0, 0.5);
  --toggle-bg: #342f29;
  --toggle-bg-hover: #3d3730;
}
```

- [ ] **Step 2: Update `.theme-toggle` styles**

Replace the `.theme-toggle` and `.theme-toggle:hover` blocks:

```css
.theme-toggle {
  position: fixed;
  bottom: 1rem;
  right: 1rem;
  z-index: 400;
  background: var(--toggle-bg);
  border: none;
  border-radius: 6px;
  padding: 3px;
  cursor: pointer;
  transition: background-color 0.15s;
}

.theme-toggle:hover {
  background: var(--toggle-bg-hover);
}
```

- [ ] **Step 3: Verify in browser**

Run: `cd docs && hugo server -D`

Check: the toggle button at bottom-right has a linen background in light mode,
`#342f29` in dark mode, and deepens on hover. Border-radius is 6px.
The existing house/sun/moon still renders (we haven't changed those yet).

- [ ] **Step 4: Commit**

```
docs(website): add background affordance to theme toggle

The toggle button was invisible chrome (no background, no border).
Add a --toggle-bg variable with light (#f0ebe4) and dark (#342f29)
variants, border-radius: 6px matching the interactive element tier,
and hover states that shift one step deeper/lighter.
```

---

### Task 2: Extract House Shape Partial and Reshape Toggle

**Files:**
- Create: `docs/layouts/partials/house-shape.html`
- Modify: `docs/layouts/partials/sidebar.html:9-12` (use shared partial)
- Modify: `docs/layouts/partials/theme-toggle.html:17` (use shared partial with scale transform)

- [ ] **Step 1: Create the shared house shape partial**

Create `docs/layouts/partials/house-shape.html`:

```html
{{- /* Copyright 2026 Phillip Cloud */ -}}
{{- /* Licensed under the Apache License, Version 2.0 */ -}}

{{- /* Canonical house shape — used by sidebar and theme toggle.
       viewBox assumed: 0 0 32 32, center-bottom at (16, 28).
       Callers set .fill and .door fill colors. */ -}}
<polygon points="16,4 2,16 6,16 6,28 26,28 26,16 30,16" fill="{{ .fill }}"/>
<rect x="13" y="19" width="6" height="9" fill="{{ .door }}"/>
```

- [ ] **Step 2: Update sidebar to use the partial**

In `docs/layouts/partials/sidebar.html`, replace lines 9-12:

```html
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" width="18" height="18">
        <polygon points="16,4 2,16 6,16 6,28 26,28 26,16 30,16" fill="currentColor"/>
        <rect x="13" y="19" width="6" height="9" fill="var(--cream, #faf6f1)"/>
      </svg>
```

with:

```html
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" width="18" height="18">
        {{- partial "house-shape.html" (dict "fill" "currentColor" "door" "var(--cream, #faf6f1)") -}}
      </svg>
```

- [ ] **Step 3: Update theme toggle to use the partial with scale transform**

In `docs/layouts/partials/theme-toggle.html`, replace line 17:

```html
    <path class="house-body" fill-rule="evenodd" d="M20,14 L11,22 L13,22 L13,32 L27,32 L27,22 L29,22 Z M18,26 L22,26 L22,32 L18,32 Z" fill="var(--charcoal-soft)"/>
```

with:

```html
    <g transform="translate(20,32) scale(0.7) translate(-16,-28)">
      {{- partial "house-shape.html" (dict "fill" "var(--charcoal-soft)" "door" "var(--toggle-bg)") -}}
    </g>
```

The transform chain: (1) move origin to sidebar center-bottom (16,28),
(2) scale by 0.7, (3) reposition to toggle center-bottom (20,32). This
produces the exact same coordinates as the P=0.7 derivation table in the
spec, but the polygon points are defined once.

The door uses `var(--toggle-bg)` so it matches the button background and reads
as a cutout.

- [ ] **Step 4: Verify in browser**

Run: `cd docs && hugo server -D`

Check:
- Sidebar house icon looks identical to before (same polygon, same fills)
- Toggle house now has the same steep roof and wide body as the sidebar
- Door is visible as a lighter rectangle matching the button background
- Compare side-by-side with the sidebar — proportions match exactly

- [ ] **Step 5: Commit**

```
docs(website): extract house shape partial, reshape toggle to match sidebar

The house polygon was duplicated with different coordinates in the
sidebar and theme toggle. Extract into a shared house-shape.html
partial. The sidebar uses it directly; the toggle wraps it in an SVG
transform (scale 0.7, repositioned to center-bottom 20,32) so the
proportions are derived, not hardcoded. Door uses --toggle-bg fill.
```

---

### Task 3: Arc Celestial Transitions

This task restructures the SVG, replaces all animation CSS, and simplifies the
JS in one go. These three files are tightly coupled — changing the SVG class
names without updating the CSS leaves a broken intermediate state.

**Files:**
- Modify: `docs/layouts/partials/theme-toggle.html` (full SVG rewrite)
- Modify: `docs/static/css/variables.css:59-121` (replace all animation CSS)
- Modify: `docs/static/js/theme.js` (remove sun-drift tracking)

- [ ] **Step 1: Rewrite the SVG in theme-toggle.html**

Replace the entire file with this structure. Key changes:
- Sun moved to resting position cx=30, cy=8 (was cx=20, cy=31)
- Both celestial bodies wrapped in nested `<g>` for split-axis transitions
- House uses the shared partial with scale transform (from Task 2)
- Moon cutout uses `var(--toggle-bg)` to match button background

```html
{{- /* Copyright 2026 Phillip Cloud */ -}}
{{- /* Licensed under the Apache License, Version 2.0 */ -}}

<button type="button" class="theme-toggle" onclick="toggleTheme()" aria-label="Toggle dark mode">
  <svg class="theme-house" viewBox="0 0 40 32" width="30" height="24" aria-hidden="true">
    <g class="sky-stars">
      <circle class="star star-1" cx="4" cy="4" r="0.6" fill="var(--charcoal)"/>
      <circle class="star star-2" cx="15" cy="2" r="0.4" fill="var(--charcoal)"/>
      <circle class="star star-3" cx="36" cy="3" r="0.5" fill="var(--charcoal)"/>
      <circle class="star star-4" cx="8" cy="11" r="0.35" fill="var(--charcoal)"/>
      <circle class="star star-5" cx="33" cy="9" r="0.3" fill="var(--charcoal)"/>
    </g>
    <g transform="translate(20,32) scale(0.7) translate(-16,-28)">
      {{- partial "house-shape.html" (dict "fill" "var(--charcoal-soft)" "door" "var(--toggle-bg)") -}}
    </g>
    <g class="sun-x">
      <g class="sun-y">
        <circle cx="30" cy="8" r="5" fill="var(--terracotta)" opacity="0.35"/>
        <circle cx="30" cy="8" r="3.2" fill="var(--terracotta)" opacity="0.55"/>
      </g>
    </g>
    <g class="moon-x">
      <g class="moon-y">
        <circle cx="10" cy="8" r="3.5" fill="var(--charcoal)"/>
        <circle cx="11.8" cy="6.5" r="2.8" fill="var(--toggle-bg)"/>
      </g>
    </g>
  </svg>
</button>
```

- [ ] **Step 2: Replace all animation CSS in variables.css**

Remove everything from `.theme-house` (line 59) through the end of the
`prefers-reduced-motion` block (line 121). Replace with:

```css
.theme-house {
  display: block;
}

/* --- Sun arc: rests upper-right in light, hides behind house in dark --- */

.sun-x {
  transition: transform 0.6s ease-out;
}

.sun-y {
  transition: transform 0.6s ease-in, opacity 0.4s ease-in;
}

[data-theme="dark"] .sun-x {
  transform: translateX(-10px);
}

[data-theme="dark"] .sun-y {
  transform: translateY(22px);
  opacity: 0;
}

/* --- Moon arc: rests upper-left in dark, hides behind house in light --- */

.moon-x {
  transform: translateX(10px);
  transition: transform 0.6s ease-out;
}

.moon-y {
  transform: translateY(22px);
  opacity: 0;
  transition: transform 0.6s ease-in, opacity 0.4s ease-in;
}

[data-theme="dark"] .moon-x {
  transform: translateX(0);
  transition: transform 0.6s ease-out 0.15s;
}

[data-theme="dark"] .moon-y {
  transform: translateY(0);
  opacity: 1;
  transition: transform 0.6s ease-out 0.15s, opacity 0.4s ease-out 0.15s;
}

/* --- Stars --- */

@keyframes twinkle {
  0%, 100% { opacity: 0.2; }
  50% { opacity: 1; }
}

.sky-stars { opacity: 0; transition: opacity 0.5s; }
[data-theme="dark"] .sky-stars { opacity: 1; }

.star { opacity: 0.2; }
.star-1 { animation: twinkle 2s ease-in-out infinite; }
.star-2 { animation: twinkle 2s ease-in-out 0.5s infinite; }
.star-3 { animation: twinkle 2s ease-in-out 1s infinite; }
.star-4 { animation: twinkle 2s ease-in-out 1.5s infinite; }
.star-5 { animation: twinkle 2s ease-in-out 0.8s infinite; }

/* --- Reduced motion --- */

@media (prefers-reduced-motion: reduce) {
  .sun-x, .sun-y, .moon-x, .moon-y { transition: none; }
  .sky-stars { transition: none; }
  .star { animation: none; }
  [data-theme="dark"] .sun-y { opacity: 0; }
  .moon-y { opacity: 0; }
  [data-theme="dark"] .moon-y { opacity: 1; }
}
```

**How the arc works:** Each celestial body has two wrapper `<g>` elements. The
outer (`*-x`) transitions `translateX`, the inner (`*-y`) transitions
`translateY`. Different easing curves on each axis produce a curved path:

- **Sun setting** (light→dark): X uses `ease-out` (fast horizontal start), Y
  uses `ease-in` (accelerating drop). The sun sweeps left, then plunges.
- **Moon rising** (150ms delay): Both axes use `ease-out`. The moon launches
  up and drifts sideways in a natural arc.
- The stagger prevents the sky from ever being empty mid-transition.

- [ ] **Step 3: Simplify theme.js**

Replace the full contents of `docs/static/js/theme.js` with:

```javascript
// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

(() => {
  var stored = localStorage.getItem('theme');
  var prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
  var isDark = stored === 'dark' || (!stored && prefersDark);
  if (isDark) {
    document.documentElement.setAttribute('data-theme', 'dark');
  }

  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
    if (localStorage.getItem('theme')) return;
    if (e.matches) {
      document.documentElement.setAttribute('data-theme', 'dark');
    } else {
      document.documentElement.removeAttribute('data-theme');
    }
    document.dispatchEvent(new CustomEvent('theme-changed'));
  });

  window.toggleTheme = () => {
    var wasDark = document.documentElement.getAttribute('data-theme') === 'dark';
    if (wasDark) {
      document.documentElement.removeAttribute('data-theme');
      localStorage.setItem('theme', 'light');
    } else {
      document.documentElement.setAttribute('data-theme', 'dark');
      localStorage.setItem('theme', 'dark');
    }
    document.dispatchEvent(new CustomEvent('theme-changed'));
  };
})();
```

Removed:
- `sessionStorage` sun-light-seen tracking
- `sun-played` class management
- `animationend` listener

Preserved:
- `localStorage` theme persistence
- `prefers-color-scheme` system theme listener
- `toggleTheme()` function
- `theme-changed` custom event for Mermaid diagram re-rendering

- [ ] **Step 4: Verify arc animation in browser**

Run: `cd docs && hugo server -D`

Check these scenarios:
1. **Light → dark click**: Sun arcs down-left behind house (fading), moon arcs
   up into sky from behind house (appearing), stars fade in.
2. **Dark → light click**: Moon arcs down behind house (fading), sun arcs up
   into sky (appearing), stars fade out.
3. **Light page load**: Sun visible upper-right, no transition on load.
4. **Dark page load**: Moon visible upper-left, stars twinkling, no transition.
5. **System theme change**: Follows same transitions.
6. **Reduced motion**: Enable in DevTools Rendering panel → instant swap.

- [ ] **Step 5: Tune arc values if needed**

The `translateX`/`translateY` values and easing curves may need visual tuning:
- If the arc is too straight, increase the difference between X and Y easings
- If celestial bodies visibly overlap the house, increase opacity fade speed
  or adjust the hidden-position translate values
- If the stagger feels wrong, adjust the 0.15s delay on the moon

- [ ] **Step 6: Commit**

```
docs(website): add arc transitions for celestial bodies

Replace the old sun-drift @keyframes and vertical moon slide with
split-axis CSS transitions. Nested <g> wrappers transition translateX
and translateY with different easing curves, creating natural arc
paths -- sun sweeps upper-right, moon sweeps upper-left, each sets
behind the house center. 150ms stagger keeps the sky occupied
mid-transition. Simplify theme.js by removing the sun-drift
animation tracking (sessionStorage, sun-played class, animationend
listener) since CSS transitions don't need it.
```

---

### Task 4: Cross-Page Smoke Test

- [ ] **Step 1: Full verification matrix**

Run: `cd docs && hugo server -D`

| Page        | Light load | Dark load | L→D arc | D→L arc | Hover | Door cutout |
|-------------|-----------|-----------|---------|---------|-------|-------------|
| Homepage    |           |           |         |         |       |             |
| Docs index  |           |           |         |         |       |             |
| Blog index  |           |           |         |         |       |             |
| Blog post   |           |           |         |         |       |             |

For each cell, verify:
- House shape matches sidebar proportions (steep roof, wide body)
- Correct celestial body visible at resting position
- Arc animation curves naturally (not a straight line)
- No celestial body visibly overlaps the house silhouette
- Button background is correct color, deepens on hover
- Door cutout color matches button background in both modes
- Stars twinkle in dark mode only
- `border-radius: 6px` visually matches search input and CTA buttons

- [ ] **Step 2: Reduced motion**

Enable "Prefers reduced motion" in DevTools Rendering panel. Toggle theme —
instant swap, no arcs, no fades.

- [ ] **Step 3: Theme persistence**

1. Set dark mode, navigate to another page — stays dark.
2. Reload — stays dark.
3. Clear localStorage, reload — follows system preference.

- [ ] **Step 4: Fix any issues found, commit fixes**
