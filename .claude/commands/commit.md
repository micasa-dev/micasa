---
description: "Use for every commit in this repo. Covers the conventional commit types that drive releases and the CI trigger phrases to avoid."
---
<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

Create a commit following this repo's conventions.

## Before committing

Run `/pre-commit-check`. The git hooks then run on `git commit`.

## Commit message format

Use conventional commits with scopes:

```
<type>(<scope>): <description>

[optional body]

[closes #<issue>]
```

## Type rules

semantic-release reads the type to decide the next version
(`.releaserc.json`).

- `feat:` -- new user-facing feature (triggers a minor version bump)
- `fix:` -- user-facing bug fix only (triggers a patch bump). Never use
  `fix:` or `fix(test):` for commits that only fix a broken test
- `perf:` -- user-visible speed-up (triggers a patch bump)
- `refactor:` -- code change with no behavior change
- `test:` or `chore(test):` -- test-only changes
- `ci:` -- CI workflow changes (not `fix:`)
- `docs:` -- documentation changes
- `docs(website):` -- website/Hugo content (not `feat(website):` to avoid
  triggering version bumps)
- `chore:` -- maintenance, deps, tooling

## Other rules

- Reference the issue number: `closes #42`, `fixes #42`
- Do not amend unless explicitly requested
- Do not use `--no-verify` under any circumstances

## Avoid CI trigger phrases

The tokens `[skip ci]`, `[ci skip]`, `[no ci]`, `[skip actions]`, and
`[actions skip]` suppress CI runs. Never include them in commit messages.
When referring to the mechanism, paraphrase instead of writing the literal
token.
