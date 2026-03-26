<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Website Dark Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add dark mode with a toggle to all three Hugo site surfaces (homepage, docs, blog), respecting OS preference with localStorage override.

**Architecture:** CSS custom properties in `variables.css` already control all colors. A `[data-theme="dark"]` selector on `<html>` overrides those properties, so all three stylesheets inherit dark colors without changing selectors. A tiny inline script in `<head>` prevents flash-of-wrong-theme. A toggle button in each header flips the attribute and persists to localStorage.

**Tech Stack:** Vanilla CSS custom properties, vanilla JS (~30 lines), Hugo templates.

---

### Task 1: Dark palette variables

**Files:**
- Modify: `docs/static/css/variables.css`

The current variables only define light colors. We need dark overrides plus two new variables for code blocks (since `--charcoal` inverts but code block backgrounds should stay dark).

- [ ] **Step 1: Add new code-block variables and dark mode overrides to variables.css**

Replace the entire file with:

```css
/* Copyright 2026 Phillip Cloud */
/* Licensed under the Apache License, Version 2.0 */

/* Shared palette -- imported by website.css, docs.css, and blog.css */

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
}
```

- [ ] **Step 2: Verify file looks correct**

Run: `cat docs/static/css/variables.css`

---

### Task 2: Update stylesheets to use new variables

**Files:**
- Modify: `docs/static/css/website.css` (lines 98-99, 297)
- Modify: `docs/static/css/docs.css` (lines 128-129, 167-168, 567-569, 731)
- Modify: `docs/static/css/blog.css` (lines 236-237, 265-266)

Several places hardcode colors that should use the new variables: `pre` backgrounds use `var(--charcoal)` (which now flips), box shadows use hardcoded rgba, and the search overlay uses a hardcoded backdrop color.

- [ ] **Step 1: Update website.css pre blocks and shadows**

In `docs/static/css/website.css`, change `pre` background and text color (around line 98):

```css
pre {
  background: var(--code-bg);
  color: var(--code-text);
  /* rest unchanged */
}
```

Change `.demo-video` box-shadow (around line 297):

```css
.demo-video {
  /* ... */
  box-shadow: 0 4px 24px var(--shadow-color);
}
```

- [ ] **Step 2: Update docs.css pre blocks, search overlay, and shadows**

In `docs/static/css/docs.css`:

Change `.search-overlay` backdrop (line 128):
```css
.search-overlay {
  /* ... */
  background: var(--overlay-bg);
  /* ... */
}
```

Change `.search-modal` box-shadow (around line 168):
```css
.search-modal {
  /* ... */
  box-shadow: 0 12px 40px var(--shadow-color);
  /* ... */
}
```

Change `.docs-main pre` background and text (around line 567):
```css
.docs-main pre {
  background: var(--code-bg);
  color: var(--code-text);
  /* rest unchanged */
}
```

Change `.docs-main img, .docs-main video` shadow (around line 731):
```css
.docs-main img,
.docs-main video {
  /* ... */
  box-shadow: 0 4px 20px var(--shadow-color);
}
```

Also change `.docs-main td code, .docs-main td kbd` (around line 626-630):
```css
.docs-main td code,
.docs-main td kbd {
  color: var(--terracotta);
  background: rgba(192, 94, 60, 0.06);
}
```
This one is fine — the rgba terracotta tint works on both light and dark backgrounds.

- [ ] **Step 3: Update blog.css pre blocks and shadows**

In `docs/static/css/blog.css`:

Change `.blog-post-body img, .blog-post-body video` shadow (around line 236):
```css
.blog-post-body img,
.blog-post-body video {
  /* ... */
  box-shadow: 0 4px 20px var(--shadow-color);
}
```

Change `.blog-post-body pre` background and text (around line 265):
```css
.blog-post-body pre {
  background: var(--code-bg);
  color: var(--code-text);
  /* rest unchanged */
}
```

---

### Task 3: Theme toggle script

**Files:**
- Create: `docs/static/js/theme.js`

This script does three things: (1) apply saved/system theme on load, (2) provide a toggle function, (3) dispatch a custom event so other scripts can react.

- [ ] **Step 1: Create theme.js**

```js
// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

(function () {
  var stored = localStorage.getItem('theme');
  var prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
  if (stored === 'dark' || (!stored && prefersDark)) {
    document.documentElement.setAttribute('data-theme', 'dark');
  }

  window.toggleTheme = function () {
    var isDark = document.documentElement.getAttribute('data-theme') === 'dark';
    if (isDark) {
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

This script must be included synchronously in `<head>` (before body renders) on all three templates to prevent flash.

---

### Task 4: Add toggle to homepage

**Files:**
- Modify: `docs/layouts/index.html`

The homepage has no nav bar. The toggle goes into the hero CTA area alongside the existing buttons, or as a small button in the top-right. Given the design decision (header nav), we add a minimal fixed-position header bar to the homepage.

Actually, re-reading the homepage — it has no nav bar at all. The hero section IS the top. The simplest approach: add a small fixed toggle button in the top-right corner, styled consistently. On docs/blog (which have nav bars), it goes in the nav.

Wait — the design decision was "header nav bar." The homepage doesn't have a header nav bar. Two options: (a) add a minimal nav bar to the homepage, or (b) put the toggle button in the footer-links area. Let's keep it simple and add a small fixed toggle button top-right on the homepage, matching the docs search-hint style.

- [ ] **Step 1: Add theme.js script to head**

In `docs/layouts/index.html`, after the `<link>` tags and before `</head>`, add:

```html
  <script src="{{ "js/theme.js" | relURL }}"></script>
```

- [ ] **Step 2: Add toggle button after the opening body tag**

After `<body>`, before the hero section, add:

```html
<button class="theme-toggle" onclick="toggleTheme()" aria-label="Toggle dark mode">
  <span class="theme-icon-light">&#9789;</span>
  <span class="theme-icon-dark">&#9788;</span>
</button>
```

- [ ] **Step 3: Add toggle button CSS to website.css**

Add before the responsive section at the end of `docs/static/css/website.css`:

```css
/* --- Theme toggle --- */
.theme-toggle {
  position: fixed;
  top: 0.75rem;
  right: 1rem;
  z-index: 400;
  background: var(--linen);
  border: 1px solid var(--rule);
  border-radius: 4px;
  padding: 0.2rem 0.5rem;
  cursor: pointer;
  font-size: 1rem;
  line-height: 1;
  color: var(--charcoal);
  transition: border-color 0.15s;
}

.theme-toggle:hover {
  border-color: var(--warm-gray);
}

.theme-icon-dark { display: none; }
[data-theme="dark"] .theme-icon-light { display: none; }
[data-theme="dark"] .theme-icon-dark { display: inline; }
```

---

### Task 5: Add toggle to docs

**Files:**
- Modify: `docs/layouts/_default/baseof.html`
- Modify: `docs/static/css/docs.css`

The docs have a topbar (visible on mobile) and a sidebar (visible on desktop). The toggle goes in the topbar nav on mobile and as a fixed button matching the search-hint on desktop.

- [ ] **Step 1: Add theme.js script to head**

In `docs/layouts/_default/baseof.html`, add before `</head>`:

```html
  <script src="{{ "js/theme.js" | relURL }}"></script>
```

- [ ] **Step 2: Add toggle to the docs topbar nav**

In the `.docs-topbar-nav` section (around line 33), add the toggle after the GitHub link:

```html
  <nav class="docs-topbar-nav">
    <a href="{{ "blog/" | relURL }}">Blog</a>
    <a href="https://github.com/micasa-dev/micasa">GitHub</a>
    <button class="theme-toggle-inline" onclick="toggleTheme()" aria-label="Toggle dark mode">
      <span class="theme-icon-light">&#9789;</span>
      <span class="theme-icon-dark">&#9788;</span>
    </button>
  </nav>
```

- [ ] **Step 3: Add a fixed toggle button for desktop (alongside search hint)**

After the `.search-hint` button (around line 40), add:

```html
<button class="theme-toggle" onclick="toggleTheme()" aria-label="Toggle dark mode">
  <span class="theme-icon-light">&#9789;</span>
  <span class="theme-icon-dark">&#9788;</span>
</button>
```

- [ ] **Step 4: Add toggle CSS to docs.css**

Add before the responsive section in `docs/static/css/docs.css`:

```css
/* ── Theme toggle ─────────────────────────────────────────── */

.theme-toggle {
  position: fixed;
  top: 0.75rem;
  right: 6rem;
  z-index: 400;
  display: inline-flex;
  align-items: center;
  padding: 0.2rem 0.5rem;
  font-size: 1rem;
  line-height: 1;
  color: var(--warm-gray);
  background: var(--cream);
  border: 1px solid var(--rule);
  border-radius: 4px;
  cursor: pointer;
  transition: color 0.15s, border-color 0.15s;
}

.theme-toggle:hover {
  color: var(--charcoal);
  border-color: var(--warm-gray);
}

.theme-toggle-inline {
  background: none;
  border: none;
  cursor: pointer;
  font-size: 1rem;
  line-height: 1;
  padding: 0;
  color: var(--warm-gray);
  transition: color 0.15s;
}

.theme-toggle-inline:hover {
  color: var(--terracotta);
}

.theme-icon-dark { display: none; }
[data-theme="dark"] .theme-icon-light { display: none; }
[data-theme="dark"] .theme-icon-dark { display: inline; }
```

- [ ] **Step 5: Update Mermaid config to react to theme changes**

In `docs/layouts/_default/baseof.html`, update the Mermaid script block (around line 173) to detect theme and re-initialize on change:

```html
<script type="module">
import mermaid from 'mermaid';

function getMermaidTheme() {
  var dark = document.documentElement.getAttribute('data-theme') === 'dark';
  return {
    startOnLoad: false,
    theme: 'base',
    themeVariables: dark ? {
      fontFamily: '"Source Serif 4", Georgia, serif',
      fontSize: '14px',
      primaryColor: '#3d3730',
      primaryBorderColor: '#d4764e',
      primaryTextColor: '#e8ddd0',
      lineColor: '#706760',
      secondaryColor: '#242019',
      tertiaryColor: '#242019'
    } : {
      fontFamily: '"Source Serif 4", Georgia, serif',
      fontSize: '14px',
      primaryColor: '#f0ebe4',
      primaryBorderColor: '#c05e3c',
      primaryTextColor: '#2d2a26',
      lineColor: '#9e958a',
      secondaryColor: '#faf6f1',
      tertiaryColor: '#faf6f1'
    }
  };
}

mermaid.initialize(getMermaidTheme());
mermaid.run();

document.addEventListener('theme-changed', async function () {
  mermaid.initialize(getMermaidTheme());
  var els = document.querySelectorAll('.mermaid[data-processed]');
  for (var i = 0; i < els.length; i++) {
    var el = els[i];
    var source = el.getAttribute('data-original-source');
    if (!source) continue;
    el.removeAttribute('data-processed');
    el.innerHTML = source;
  }
  await mermaid.run();
});
</script>
```

Also, update the Mermaid render-codeblock template to preserve the original source. Check `docs/layouts/_default/_markup/render-codeblock-mermaid.html` and ensure the original mermaid source is stored in a data attribute. If it renders as:

```html
<pre class="mermaid">{{ .Inner }}</pre>
```

Change to:

```html
<pre class="mermaid" data-original-source="{{ .Inner | htmlEscape }}">{{ .Inner }}</pre>
```

- [ ] **Step 6: Hide the desktop fixed toggle on mobile (topbar has inline one)**

In the responsive section of `docs/static/css/docs.css`, inside the `@media (max-width: 800px)` block, add:

```css
  .theme-toggle {
    display: none;
  }
```

---

### Task 6: Add toggle to blog

**Files:**
- Modify: `docs/layouts/blog/baseof.html`
- Modify: `docs/static/css/blog.css`

- [ ] **Step 1: Add theme.js script to head**

In `docs/layouts/blog/baseof.html`, add before `</head>`:

```html
  <script src="{{ "js/theme.js" | relURL }}"></script>
```

- [ ] **Step 2: Add toggle button to blog nav**

In the `.blog-nav` section (around line 27), add after the GitHub link:

```html
    <nav class="blog-nav">
      <a href="{{ "blog/" | relURL }}">Blog</a>
      <a href="{{ "docs/" | relURL }}">Docs</a>
      <a href="https://github.com/micasa-dev/micasa">GitHub</a>
      <button class="theme-toggle-inline" onclick="toggleTheme()" aria-label="Toggle dark mode">
        <span class="theme-icon-light">&#9789;</span>
        <span class="theme-icon-dark">&#9788;</span>
      </button>
    </nav>
```

- [ ] **Step 3: Add toggle CSS to blog.css**

Add before the responsive section in `docs/static/css/blog.css`:

```css
/* -- Theme toggle ------------------------------------------- */

.theme-toggle-inline {
  background: none;
  border: none;
  cursor: pointer;
  font-size: 1rem;
  line-height: 1;
  padding: 0;
  color: var(--warm-gray);
  transition: color 0.15s;
}

.theme-toggle-inline:hover {
  color: var(--terracotta);
}

.theme-icon-dark { display: none; }
[data-theme="dark"] .theme-icon-light { display: none; }
[data-theme="dark"] .theme-icon-dark { display: inline; }
```

---

### Task 7: Update house-crumble.js to use CSS variables

**Files:**
- Modify: `docs/static/js/house-crumble.js`

The script has hardcoded ember colors and a smoke color. The ember colors (warm oranges) work on both light and dark backgrounds — they're glowing effects. But the rubble smoke color on line 244 (`#9e958a`) should use the CSS variable.

- [ ] **Step 1: Read smoke color from CSS variable**

In `docs/static/js/house-crumble.js`, change line 244 from:

```js
      el.style.color = '#9e958a';
```

to:

```js
      el.style.color = getComputedStyle(document.documentElement).getPropertyValue('--warm-gray').trim();
```

The ember colors (`EMBER_BASE`, `EMBER_COLORS`) are intentionally warm orange glows that look good on both backgrounds, so leave them as constants.

---

### Task 8: Dark syntax highlighting

**Files:**
- Modify: `docs/static/css/syntax.css`

The current Gruvbox syntax theme uses colors designed for dark backgrounds. Since code blocks already have `--code-bg` (dark in both modes), the syntax colors work well as-is. However, the line highlight color and line number color may need adjustment for dark mode.

- [ ] **Step 1: Verify syntax colors work on both themes**

The Gruvbox syntax colors (`#fe8019`, `#b8bb26`, `#fabd2f`, etc.) are designed for dark backgrounds. Since `--code-bg` stays dark in both light and dark mode, no changes are needed to syntax.css.

Run: `echo "No changes needed to syntax.css — Gruvbox colors work on dark code-bg in both modes"`

---

### Task 9: Verify and test

- [ ] **Step 1: Build the Hugo site**

Run: `cd docs && hugo --minify`

Verify no build errors.

- [ ] **Step 2: Manual verification checklist**

Run the Hugo dev server and check:
- Homepage: toggle visible top-right, switches between sun/moon
- Homepage: colors change correctly, pre blocks stay dark
- Homepage: house ASCII art still visible in both modes
- Docs: toggle visible next to search hint on desktop
- Docs: toggle in topbar on mobile
- Docs: sidebar, search modal, code blocks all correct
- Docs: Mermaid diagrams re-render on theme switch
- Blog: toggle in header nav
- Blog: code blocks, body text, links all correct
- Both modes: localStorage persists across page reload
- Both modes: respects OS preference when no stored value

- [ ] **Step 3: Commit**

```
feat(docs): add dark mode with toggle to website

Warm dark palette (#242019 base) that preserves the earthy brand
identity. Toggle in header nav on all three surfaces (homepage,
docs, blog). Respects prefers-color-scheme with localStorage
override. Mermaid diagrams re-render on theme switch.
```

---

## Notes

- **Pagefind search UI**: Already uses CSS custom properties via `--pagefind-ui-*` set in the `.search-modal` selector, which references our palette variables. Should adapt automatically.
- **No image changes**: Screenshots/videos look fine on both backgrounds due to border-radius and shadow.
- **Chimney smoke**: Already uses `var(--warm-gray)` via the `.smoke-particle` CSS class — no JS changes needed.
- **Fonts**: No changes needed — serif/mono fonts look good on both backgrounds.
