---
description: "Use after UI or UX changes to re-record the README demo video and the docs screenshots."
---
<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

Re-record the demo and/or the screenshot tapes after UI/UX changes.

## Demo

`nix run '.#record-demo'` records `docs/tapes/demo.tape` to
`videos/demo.webm` and converts it to `images/demo.webp`, which the README
shows. Commit both files with the feature.

## Screenshots

Screenshot tapes live in `docs/tapes/`; `capture-screenshots` writes WebP
files to `docs/static/images/`. Each tape takes about 2 minutes, so when
iterating on themes or capture settings:

1. Capture one tape first and inspect it:
   `nix run '.#capture-screenshots' -- dashboard`
2. Once it looks right, capture them all: `nix run '.#capture-screenshots'`

With no arguments it captures every tape except `demo` and the animated
ones (`using-*`, `extraction`). Re-record those with
`nix run '.#record-animated'`.
