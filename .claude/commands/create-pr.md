---
description: "Use when opening or updating a pull request in this repo. Adds micasa's merge, label, and issue-closing rules to the general pull request workflow."
---
<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

micasa's rules for pull requests. Write the title and body with your
general pull request workflow, then apply these.

## Before creating

1. Push the branch: `git push -u origin HEAD`
2. Run `/pre-commit-check` if you haven't already

## Rules

- **Rebase merges only**: merge with `gh pr merge --rebase`. No merge
  commits, no squash merges.
- **Labels**: add the type and scope labels that AGENTS.md lists.
- **No "Test plan" section**: CI runs the tests, lint, vet, and build.
  Add one only for checks that can't be automated, such as a visual check
  of the TUI.
- **Reproduction steps**: a bug-fix PR lists numbered steps to reproduce
  the bug.
- **Keep the description current**: after pushing more commits, re-read
  the title and body (`gh pr view`) and update them to match the branch.
- **Leave AGENTS.md out**: when AGENTS.md changes ride along with other
  work, don't mention them. Mention them only in a PR about agent rules
  alone.
- **No CI trigger phrases**: never put `[skip ci]`, `[ci skip]`, or
  similar tokens in the title or body.

## Closing issues

Before creating the PR, search for open issues it resolves:

```
gh issue list --repo micasa-dev/micasa --search "<relevant keywords>" --state open
```

Add a `closes #<number>` line to the body for each one, so GitHub closes
the issue on merge. Use the same `closes` syntax in commit messages when it
applies.
