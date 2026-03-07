<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Speed up pre-commit hook (#648)

## Problem

All 15 pre-commit hooks run serially at the `pre-commit` stage. Several are
slow whole-program analyses (deadcode, govulncheck, osv-scanner, golangci-lint)
that add significant latency to every commit.

## Design

Split hooks into two tiers by moving slow analysis tools to the `pre-push`
stage. Fast formatters and linters stay on `pre-commit` for immediate feedback.

### Pre-commit (fast, every commit)

- golines (Go formatter)
- nixfmt (Nix formatter)
- biome (JS/JSON formatter+linter)
- taplo (TOML formatter)
- license-header (license check)
- actionlint (GitHub Actions linter)
- statix (Nix linter)
- deadnix (dead Nix code)
- go-mod-tidy (go.mod staleness)
- vendor-hash-check (vendorHash staleness)
### Pre-push (slow, before push)

- deadcode (whole-program dead code analysis)
- govulncheck (call-graph vulnerability analysis)
- osv-scanner (dependency vulnerability scan)
- golangci-lint (full Go lint suite)
- go-generate-check (compiles and runs go generate)

### `run-pre-commit` wrapper

Updated to run both stages so `nix run '.#pre-commit'` still covers
everything:

```
pre-commit run --all-files
pre-commit run --all-files --hook-stage pre-push
```

### Shell hook

git-hooks.nix auto-detects the stages used across all hooks and installs
both `pre-commit` and `pre-push` git hooks via the generated `shellHook`.

## Trade-offs

- Commits no longer catch lint/deadcode issues immediately. This is
  acceptable because CI still catches everything, and `pre-push` runs
  before code reaches the remote.
- Users can still run `nix run '.#pre-commit'` for the full battery.
