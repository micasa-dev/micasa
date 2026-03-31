<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

Create a pull request for the current branch.

## Before creating

1. Ensure all commits are pushed (`git push -u origin HEAD`)
2. Run `/pre-commit-check` if you haven't already

## PR conventions

- **Use a HEREDOC for the body**: Pass the PR body via a HEREDOC to
  avoid shell-quoting issues. Do not write temporary files to `/tmp`.
  Example: `gh pr create --title "..." --body "$(cat <<'EOF'` ...
  `EOF` `)"`
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
  in the PR title or body.

## Closing issues

Before creating the PR, search for open issues that would be closed by it:

```
gh issue list --repo micasa-dev/micasa --search "<relevant keywords>" --state open
```

Add `closes #<number>` lines to the PR body for each matching issue so
GitHub auto-closes them on merge. Use the commit message `closes` syntax
too when applicable.

## PR body format

```markdown
## Summary

- Concise bullet points describing what changed and why

closes #<issue>
```

Keep it short. The diff tells the story.
