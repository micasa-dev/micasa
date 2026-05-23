+++
title = "Contributing"
weight = 5
description = "How to contribute to micasa."
linkTitle = "Contributing"
+++

PRs welcome! Here's how to get set up and what to expect.

## Open an issue first

Bug fixes, new features, refactors; all welcome, but **please open an issue
before writing code.** A quick conversation up front avoids wasted effort on
both sides. Describe what you want to change and why; we'll hash out the
approach before you invest time in a PR.

Exceptions: typo fixes, doc clarifications, and other trivially obvious
changes can go straight to a PR.

## AI-assisted code

micasa is developed with AI coding agents, so AI-assisted
contributions are welcome; with one hard requirement: **you must understand
and stand behind the code you submit.**

What that means in practice:

- **Hand-written code**; always welcome.
- **AI-assisted code you've reviewed and curated**; also welcome. Use
  whatever tools help you write better code faster.
- **Bulk AI-generated PRs with no human curation**; not welcome. If a PR
  reads like unedited LLM output (verbose boilerplate, hallucinated APIs,
  changes that don't match the codebase conventions, or a suspiciously large
  diff with no clear purpose), it will be closed.

The bar is the same regardless of how the code was written: does it solve a
real problem, follow the project's patterns, and come with tests?

## Code review

PRs will likely get an initial review from an AI agent before a human looks at
them. This is an experimental workflow; the project is built with AI tooling
and reviewed with it too. A human always makes the final call on merging.

## Scope

micasa is an end-user application, not a library. PRs that refactor internals
into importable packages, add a public Go API, or otherwise repackage micasa
for use as a dependency will be closed.

## Setup

1. Fork and clone the repo
2. Enter the dev shell: `nix develop` (or install Go 1.25+ manually)
3. The dev shell auto-installs pre-commit hooks on first entry

## Pre-commit hooks

The repo uses pre-commit hooks that run automatically on `git commit`:

- **golines** + **gofumpt**: code formatting (max 100 chars/line)
- **golangci-lint**: static analysis
- **license-header**: ensures every source file has the Apache-2.0 header

If a hook fails, fix the issue and commit again. The hooks auto-fix formatting
where possible.

## Commit conventions

micasa uses [conventional commits](https://www.conventionalcommits.org/) with
scopes. Examples:

```
feat(dashboard): add spending summary section
fix(maintenance): correct next-due computation for edge case
refactor(handlers): extract shared inline edit logic
test(sort): add multi-column comparator tests
docs(website): update feature list
```

Use `docs(website):` (not `feat(website):`) for website changes to avoid
triggering version bumps.

## Dependencies

micasa is offline by design; single SQLite file, no cloud, no accounts. PRs
that add dependencies requiring network access or external services will almost
certainly be closed. If you think your case is the exception, make the argument
in the issue first.

## Code style

- **Run `go mod tidy` before committing** to keep dependencies clean
- Follow existing patterns: check how similar features are implemented
- Use the Wong colorblind-safe palette for any new colors (see `styles.go`)
- Always provide both Light and Dark variants in `lipgloss.AdaptiveColor`
- Keep type safety: avoid `any` casts, use proper types and guards
- DRY: search for existing helpers before adding new ones

## Tests

- Write tests for new features
- Don't test implementation details; test behavior
- Run `go test -shuffle=on -v ./...` to verify
- All tests must pass on Linux, macOS, and Windows

## License

By contributing, you agree that your contributions will be licensed under the
Apache License 2.0. All source files must include the copyright header (the
pre-commit hook handles this automatically).
