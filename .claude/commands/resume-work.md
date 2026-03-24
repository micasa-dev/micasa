<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

Pick up context from a previous agent session that ran out of context.

Run these steps to understand the current state before doing anything else:

1. **Check worktrees**: `git worktree list` -- identify which worktree you're
   in and what branches exist
2. **Recent git history**: `git log --oneline -20` -- see what was committed
   recently and by whom
3. **Current branch state**: `git status` and `git diff --stat` -- see any
   uncommitted work in progress
4. **Open PRs**: `gh pr list` -- check for PRs that may need updates or
   are awaiting review
5. **Open issues**: `gh issue list --repo micasa-dev/micasa` -- browse recent
   issues to understand what's being worked on
6. **Read conversation summary**: If the session includes a continuation
   summary, read it carefully for pending tasks, file locations, and
   decisions already made

After gathering context, summarize what you found and ask the user what to
work on next (or continue the pending task if one is obvious).
