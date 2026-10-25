---
description: "Use when CI jobs fail on the current branch's pull request. Finds each failing job, reads its log, and fixes the cause."
---
<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

Diagnose and fix failing CI jobs on the current branch's PR.

## 1. Identify failures

1. List the failing checks with their links:

   ```
   gh pr view --json statusCheckRollup --jq '.statusCheckRollup[] | select(.conclusion == "FAILURE") | [.name, .detailsUrl] | @tsv'
   ```

2. If nothing fails, report that and stop.

## 2. Diagnose each failure

For each failing job:

1. Take the run ID from the `detailsUrl` (`.../actions/runs/<run_id>/job/...`).
2. Fetch failed logs: `gh run view <run_id> --log-failed`
3. Read the relevant source files to understand the root cause.
4. Common failure categories:
   - **Compile errors**: Fix the Go code.
   - **Test failures**: Read the failing test, understand the assertion,
     fix the code (not the test) unless the test itself is wrong.
   - **Lint/format**: Run the linter locally, apply fixes.
   - **Nix build**: Hash mismatches need `/update-vendor-hash`. Eval
     errors need Nix expression fixes.
   - **OSV scanner**: Use `/fix-osv-finding`.
   - **Cross-platform (Windows/PowerShell)**: No `&&` in commands, no
     bash-isms, use `-bench .` not `-bench=.`.

## 3. Verify locally

Run `/pre-commit-check` to confirm the fixes work before pushing.

## 4. Push

1. `git push` (or `git push --force-with-lease` if you rebased)
2. Report what failed and what you changed. Don't poll CI. If the user asks
   you to wait for the new run, run the step 1 command once it finishes and
   loop back to step 2 for new failures.
