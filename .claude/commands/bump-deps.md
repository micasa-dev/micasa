---
description: "Use to bump every project dependency (Nix flake inputs, Go modules, Go analysis tools, GitHub Actions) and verify the build, tests, and scans."
---
<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

Bump all project dependencies to their latest versions and verify the
build, tests, and security scans still pass. This covers Nix flake inputs,
Go modules, the pinned Go analysis tools, and GitHub Actions.

Fix forward: if an update breaks the build or tests, fix the breakage
rather than pinning to the old version. Only pin as a last resort when
the fix is non-trivial and unrelated to the current work.

## Steps

### 1. Nix flake inputs

`nix flake update`

### 2. Go modules

1. `go get -u ./...`
2. `go mod tidy`

### 3. Go analysis tools

`deadcode` and `nilaway` are not built by Nix. `nix/scripts/deadcode.bash`,
`nix/scripts/nilaway.bash`, and `.github/workflows/lint.yml` run them with
`go run`/`go install` at a version pinned under a
`# renovate: datasource=go depName=<module>` comment, and Renovate updates
those pins. To bump by hand, find the latest versions:

```
go list -m -json golang.org/x/tools@latest | jq -r .Version
go list -m -json go.uber.org/nilaway@latest | jq -r .Version
```

Update every line pinned under the matching `# renovate:` comment.

### 4. GitHub Actions

For each `uses:` line pinned to a SHA in `.github/workflows/*.yml`:

1. Look up the latest release:
   `gh release view --repo <owner>/<repo> --json tagName --jq .tagName`
2. Get the commit SHA for that tag:
   `git ls-remote https://github.com/<owner>/<repo> 'refs/tags/<tag>' 'refs/tags/<tag>^{}'`
   An annotated tag prints two lines; use the SHA on the `^{}` line, which
   is the commit.
3. Update both the SHA and the version comment, preserving the
   `owner/repo@<sha> # <tag>` format.

### 5. Verification

Run these in order; fix any failures before proceeding to the next step.

1. **Vendor hash**: use `/update-vendor-hash` to fix the Nix `vendorHash`
   after `go.sum` or flake input changes
2. **Nix build**: `nix build '.#micasa'`
3. **Go build**: `go build ./...`
4. **Tests**: `go test -shuffle=on ./...`
5. **Vulnerability scan**: `nix run '.#osv-scanner'` -- if new findings
   appear, use `/fix-osv-finding`. A newer Go version may resolve
   previously ignored stdlib CVEs; remove stale `[[IgnoredVulns]]` from
   `osv-scanner.toml` when fixed
6. **Lint**: `nix run '.#pre-commit' -- --from-ref origin/main --to-ref HEAD --verbose`
7. **Deadcode**: `nix run '.#deadcode'`

## Commit

Use `/commit` with type `chore(deps)` and a message summarizing what was
bumped. Note any notable version jumps or behavioral changes in the body.
