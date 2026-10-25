---
description: "Use to build a PR branch and launch it in tmux for hands-on testing, an A/B comparison against another branch, or an automated smoke test."
---
<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Test PR

Build a PR branch, seed a test database, and launch in tmux for
interactive testing.

## Arguments

- First argument: PR number, worktree name, or branch name
- `--ab`: A/B comparison mode (optional second argument for second
  branch; defaults to main)
- `--smoke`: automated smoke test mode (non-interactive)

## Input Resolution

Resolve the input to a worktree path:

- **PR number** (all digits): run `gh pr view <number> --repo
  micasa-dev/micasa --json headRefName --jq .headRefName` to get the
  branch name, then find a matching worktree.
- **Worktree name**: check if `.claude/worktrees/<name>` exists.
- **Branch name**: check `git worktree list` for a worktree on that
  branch.

To find a worktree matching a branch name, run:

```sh
git worktree list --porcelain | rg -B2 '^branch refs/heads/<branch>$' | rg -o '^worktree (.+)' -r '$1'
```

If no worktree exists, create a temporary one:

```sh
git worktree add .claude/worktrees/test-pr-<branch> <branch>
```

Store the resolved worktree path for use in subsequent steps.

## Single-Branch Mode (default)

Used when neither `--ab` nor `--smoke` is passed.

### Build

Build the binary from the resolved worktree:

```sh
direnv exec <worktree> go build -C <worktree> -o /tmp/micasa-pr-<id> ./cmd/micasa
```

Where `<id>` is the PR number, worktree name, or branch name
(sanitized for filesystem use). Report build errors and stop if
the build fails.

### Seed database

```sh
/tmp/micasa-pr-<id> demo --seed-only /tmp/micasa-test-<id>.db
```

This creates a fresh SQLite database with demo data. Previous
databases at the same path are overwritten.

### Kill stale session

```sh
tmux -L claude-tui kill-session -t pr-<id> 2>/dev/null
```

### Launch in tmux

```sh
tmux -L claude-tui new-session -d -s pr-<id> -x 120 -y 40
tmux -L claude-tui send-keys -t pr-<id> '/tmp/micasa-pr-<id> /tmp/micasa-test-<id>.db' Enter
```

Wait 2 seconds, then capture the pane to verify the app started:

```sh
sleep 2
tmux -L claude-tui capture-pane -t pr-<id> -p
```

If the capture shows a panic or shell prompt (app exited), report
the error.

### Handoff

Print this to the user:

```
Built from <worktree> (branch <branch>)
DB seeded at /tmp/micasa-test-<id>.db

Attach with:  ! tmux -L claude-tui attach -t pr-<id>

Detach with Ctrl+B, D to return here.
Kill session: ! tmux -L claude-tui kill-session -t pr-<id>
```

## A/B Comparison Mode (`--ab`)

Builds and launches two branches side by side in a split tmux session.

### Resolve both sides

- **Left pane**: the first argument (resolved as above)
- **Right pane**: the second argument if provided (resolved the same
  way), otherwise main. For main, use the repository's main checkout
  directory (parent of `.claude/worktrees/`).

### Build both binaries

```sh
direnv exec <worktree-left> go build -C <worktree-left> -o /tmp/micasa-ab-left ./cmd/micasa
direnv exec <worktree-right> go build -C <worktree-right> -o /tmp/micasa-ab-right ./cmd/micasa
```

Build these in parallel (two Bash tool calls). Report errors and
stop if either fails.

### Seed both databases

```sh
/tmp/micasa-ab-left demo --seed-only /tmp/micasa-ab-left.db
/tmp/micasa-ab-right demo --seed-only /tmp/micasa-ab-right.db
```

### Kill stale session and launch

```sh
tmux -L claude-tui kill-session -t ab-<id> 2>/dev/null
tmux -L claude-tui new-session -d -s ab-<id> -x 240 -y 40
tmux -L claude-tui send-keys -t ab-<id> '/tmp/micasa-ab-left /tmp/micasa-ab-left.db' Enter
tmux -L claude-tui split-window -h -t ab-<id>
tmux -L claude-tui send-keys -t ab-<id> '/tmp/micasa-ab-right /tmp/micasa-ab-right.db' Enter
```

Wait 3 seconds, capture both panes to verify startup.

### Handoff

```
A/B comparison: <left-label> (left) vs <right-label> (right)
DBs: /tmp/micasa-ab-left.db, /tmp/micasa-ab-right.db

Attach with:  ! tmux -L claude-tui attach -t ab-<id>

Switch panes: Ctrl+B, Arrow
Detach: Ctrl+B, D
Kill: ! tmux -L claude-tui kill-session -t ab-<id>
```

## Smoke Test Mode (`--smoke`)

Automated non-interactive smoke test. Builds and launches the app,
runs predefined keystrokes, captures output after each, and checks
for panics or rendering failures.

### Build and launch

Same as single-branch mode. Use session name `smoke-<id>`.

### Test sequence

After the app starts (wait 3 seconds), run each test step. Between
every keystroke, wait 0.5 seconds and capture the pane. Each step
is one `tmux send-keys` call followed by one `capture-pane`.

**Tab navigation**: press `f` five times and `b` five times. Verify
each capture is non-empty and contains no "panic:" or
"runtime error".

**Dashboard overlay**: press `D` to open, capture, press `D` to
close, capture. Verify both captures are non-empty and differ from
each other.

**Row navigation**: press `j` three times, then `k` three times,
then `G` (end), then `g` (top). Verify captures are non-empty.

**House overlay**: press `Tab` to open, capture, press `Escape` to
close, capture.

**Detail view**: press `Enter` to open, capture, press `Escape` to
close, capture.

### Failure detection

After each capture, check for:
- Substring "panic:" or "runtime error" → FAIL
- Completely blank output (only whitespace) → FAIL
- App exited (shell prompt visible) → FAIL

### Report

Print a summary:

```
Smoke test: <label>
  [PASS/FAIL] Tab navigation
  [PASS/FAIL] Dashboard overlay
  [PASS/FAIL] Row navigation
  [PASS/FAIL] House overlay
  [PASS/FAIL] Detail view
  Result: N/5 passed
```

On failure, include the captured output that triggered the failure.

### Cleanup

Kill the tmux session after the smoke test completes:

```sh
tmux -L claude-tui kill-session -t smoke-<id>
```
