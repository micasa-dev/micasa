<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

Re-record the demo GIF and/or screenshot tapes after UI/UX changes.

## Demo GIF

Run `nix run '.#record-demo'` to update `videos/demo.webm` (used in README).
Commit the GIF with the feature.

## Screenshots

Screenshots live in `docs/tapes/`. When iterating on themes or capture
settings:

1. Modify the `capture-screenshots` script to only run a single capture
   (e.g. just `dashboard`) and inspect the result first
2. Only after confirming the result looks right, run the full capture
   (`nix run '.#capture-screenshots'`) -- each of the 9 screenshots takes
   ~2 min

Do not re-run all 9 screenshots just to check a theme change.
