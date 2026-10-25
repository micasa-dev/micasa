---
description: "Use when osv-scanner reports a vulnerability. Findings block commits until the dependency is fixed or the finding is shown to be unreachable."
---
<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

Remediate a vulnerability reported by `osv-scanner`.

OSV scanner findings are blockers -- they must be resolved before committing.

## Try to fix first

1. Update the affected dependency: `go get <package>@latest`
2. If it's a stdlib vuln, bump the Go version in `go.mod`
3. Run `go mod tidy`
4. Use `/update-vendor-hash` if `nix build '.#micasa'` fails with a hash
   mismatch
5. Re-run `nix run '.#osv-scanner'` to confirm the finding is resolved

## If the vuln is unreachable

If the vulnerability genuinely does not apply to micasa's usage (the affected
code path is never reached, the preconditions don't hold), add an entry to
`osv-scanner.toml`:

```toml
[[IgnoredVulns]]
id = "GHSA-xxxx-xxxx-xxxx"
reason = "micasa doesn't use the affected X feature because Y"
```

The reason must explain why the vuln is unreachable in micasa's architecture
specifically -- not "blocked on upstream" or "toolchain-level". Never dismiss
scanner output without analyzing whether the vuln is reachable.
