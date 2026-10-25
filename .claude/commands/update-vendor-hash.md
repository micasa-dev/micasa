---
description: "Use after Go dependencies change (go.mod or go.sum) to update micasa's Nix vendorHash."
---
<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

Update the Nix `vendorHash` in `nix/package.nix` after adding or changing Go
dependencies.

Steps:

1. `nix build '.#micasa'`
2. If it fails with a hash mismatch, set `vendorHash = lib.fakeHash;` in
   `nix/package.nix`. The next build prints the expected hash in its error.
3. `nix build '.#micasa'`, then replace `lib.fakeHash` with the hash on the
   `got:` line
4. `nix build '.#micasa'` again to confirm it builds cleanly

Never use `vendorHash = "";`. Nix prints a deprecation warning for it.
