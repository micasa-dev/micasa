<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

Create a commit following this repo's conventions.

## Before committing

Do not run pre-commit hooks manually -- they run automatically on
`git commit`. Just make sure the code compiles and tests pass.

## Commit message format

Use conventional commits with scopes:

```
<type>(<scope>): <description>

[optional body]

[closes #<issue>]
```

## Type rules

- `feat:` -- new user-facing feature (triggers minor version bump)
- `fix:` -- user-facing bug fix only (triggers patch bump). Never use
  `fix:` or `fix(test):` for commits that only fix a broken test
- `test:` or `chore(test):` -- test-only changes
- `ci:` -- CI workflow changes (not `fix:`)
- `docs:` -- documentation changes
- `docs(website):` -- website/Hugo content (not `feat(website):` to avoid
  triggering version bumps)
- `chore:` -- maintenance, deps, tooling

## Scope conventions

- Reference the issue number: `closes #42`, `fixes #42`
- AGENTS.md-only commits: add the standard no-ci marker to the message.
  There's nothing to build or test.
- Do not amend unless explicitly requested
- Do not use `--no-verify` under any circumstances

## Avoid CI trigger phrases

The tokens `[skip ci]`, `[ci skip]`, `[no ci]`, `[skip actions]`, and
`[actions skip]` suppress CI runs. Never include them in commit messages
unless you intend to suppress CI (e.g. AGENTS.md-only changes). When
referring to the mechanism, paraphrase instead of writing the literal token.
