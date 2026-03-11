<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

Rebase onto the latest main, address PR review feedback, and fix failing CI.

## 1. Rebase onto main

1. `git fetch origin main`
2. `git rebase origin/main`
3. If there are conflicts, resolve them, `git add` the resolved files, and
   `git rebase --continue`. Repeat until the rebase completes.

## 2. Address PR review feedback and fix CI (parallel)

These two steps are independent -- start both in parallel.

### 2a. PR review feedback

1. Fetch unresolved review threads (use the GraphQL API for threaded context):
   ```
   gh api graphql -f query='
     query($owner:String!, $repo:String!, $pr:Int!) {
       repository(owner:$owner, name:$repo) {
         pullRequest(number:$pr) {
           reviewThreads(first:100) {
             nodes {
               id
               isResolved
               comments(first:50) {
                 nodes { id databaseId author{login} body path line }
               }
             }
           }
         }
       }
     }' -f owner=cpcloud -f repo=micasa \
        -F pr="$(gh pr view --json number --jq '.number')"
   ```
2. For each **unresolved** thread:
   - Read the referenced file and line to understand the context
   - Make the requested change (or explain in a reply why not)
   - After pushing the fix, reply to the review comment using its
     `databaseId` from the query:
     `gh api repos/cpcloud/micasa/pulls/<pr>/comments/<databaseId>/replies -f body='...'`
     Explain how it was addressed (commit hash, what changed).
   - **Resolve the thread** if you are extremely confident the feedback
     has been fully addressed. Use the GraphQL mutation:
     ```
     gh api graphql -f query='
       mutation($id:ID!) { resolveReviewThread(input:{threadId:$id}) {
         thread { isResolved }
       }}' -f id=<thread_node_id>
     ```
     If there is any doubt the comment hasn't been fully addressed, leave
     the thread open for the reviewer.
3. Skip resolved threads -- they need no action.

### 2b. Fix failing CI

Use `/fix-ci` to diagnose and fix each failing job.

## 3. Push and verify

1. `git push --force-with-lease` (safe force push since we rebased)
2. Wait for CI to start: `gh pr checks --watch --fail-fast`
3. If new failures appear, loop back to step 2b.

## 4. Update PR description

After all changes are pushed, re-read the PR title and body
(`gh pr view`) and update them if they no longer match the actual changes.
