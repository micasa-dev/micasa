<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

Rebase onto the latest main, then iterate until all review feedback is
addressed and CI is green.

## 1. Rebase onto main

1. `git fetch origin main`
2. `git rebase origin/main`
3. If there are conflicts, resolve them, `git add` the resolved files, and
   `git rebase --continue`. Repeat until the rebase completes.

## 2. Review-fix loop

Repeat until there are **zero unresolved threads** and CI is green.
Squash fixes into the relevant commit (fixup + autosquash) to keep
history clean.

### 2a. Address unresolved review feedback

1. Fetch unresolved review threads:
   ```
   gh api graphql \
     -F query=@.claude/graphql/review-threads.graphql \
     -f owner=micasa-dev -f repo=micasa \
     -F pr="$(gh pr view --json number --jq '.number')"
   ```
   Filter to `isResolved == false`.
2. For each **unresolved** thread:
   - Read the referenced file and line to understand the context.
   - Make the requested change (or explain in a reply why not).
   - Reply to the review comment using its `databaseId`:
     `gh api repos/micasa-dev/micasa/pulls/<pr>/comments/<databaseId>/replies -f body='...'`
     Explain how it was addressed (commit hash, what changed).
   - **Resolve the thread**:
     ```
     gh api graphql \
       -F query=@.claude/graphql/resolve-thread.graphql \
       -f id=<thread_node_id>
     ```
     Only leave a thread open if there is genuine doubt it was fully
     addressed.
3. If there are no unresolved threads, skip to 2c.

### 2b. Fix failing CI

Use `/fix-ci` to diagnose and fix each failing job.

### 2c. Push and wait

1. `git push --force-with-lease`
2. Wait for CI: `gh pr checks --watch --fail-fast`

## 3. Update PR description

After the loop exits, re-read the PR title and body (`gh pr view`) and
update them if they no longer match the actual changes.
