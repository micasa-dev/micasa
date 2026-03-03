<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

Create a pull request for the current branch.

## Before creating

1. Ensure all commits are pushed (`git push -u origin HEAD`)
2. Run `/pre-commit-check` if you haven't already

## PR conventions

- **Use `--body-file`**: Write the PR body to a file and pass
  `--body-file` instead of `--body`. This avoids shell-quoting issues
  that silently corrupt markdown.
- **Rebase merges only**: This repo uses `gh pr merge --rebase`. No merge
  commits, no squash merges.
- **No "Test plan" section**: CI covers tests, lint, vet, and build.
  Only include a test plan for genuinely manual-only verification steps
  (e.g. visual UI/UX checks).
- **Keep descriptions in sync**: After pushing additional commits, re-read
  the PR title and body (`gh pr view`) and update them if they no longer
  match the actual changes.
- **Don't mention AGENTS.md**: When AGENTS.md changes accompany other work,
  omit them from the summary. Only mention AGENTS.md if the PR is solely
  about agent rules.
- **Avoid CI trigger phrases**: Never put `[skip ci]`, `[ci skip]`, etc.
  in the PR title or body unless you intend to suppress CI.

## PR body format

```markdown
## Summary

- Concise bullet points describing what changed and why
```

Keep it short. The diff tells the story.
