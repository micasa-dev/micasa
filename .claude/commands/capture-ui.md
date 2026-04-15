<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

Capture a screenshot or short video demonstrating a TUI feature or bugfix
for PR review.

## Arguments

Optional: a PR number (e.g. `/capture-ui 123`). When given, the PR is
checked out in an isolated worktree, micasa is built from that source, and
the capture runs against the PR's code. Without a PR number, captures run
against the current working tree.

## When to use

- After completing TUI feature or bugfix work, before or during PR creation
- To visually verify an existing PR's UI changes

## Procedure

### 0. PR mode setup (skip if no PR number)

When a PR number is provided:

```sh
pr_num=<N>
pr_branch=$(gh pr view "$pr_num" --json headRefName --jq .headRefName)
worktree_dir=".claude/worktrees/pr-$pr_num"
git worktree add "$worktree_dir" "$pr_branch"
```

Build micasa from the PR source:
```sh
CGO_ENABLED=0 go build -trimpath -o "$worktree_dir/micasa" -C "$worktree_dir" ./cmd/micasa
```

All remaining steps use `$worktree_dir/micasa` instead of `micasa` on
PATH. The tape preamble changes accordingly (see step 3).

### 1. Understand what changed

Before writing any tape, understand what the PR or recent work actually
changed in the UI. Random screenshots of unrelated screens are useless.

**PR mode:**
```sh
gh pr view <N> --json body --jq .body
gh pr diff <N> --name-only
```

Read the PR description and changed files. Identify:
- Which screen/tab/overlay was affected
- What the visual difference is (new element, changed layout, error state, etc.)
- What user interaction triggers the change
- Whether the change is even visually demonstrable (some changes like
  error handling require specific failure conditions that demo mode can't
  produce — in that case, tell the user and skip the capture)

**Normal mode:** You already know what changed because you just wrote it.
Still pause and identify the specific screen state that demonstrates it.

If the change is not visually demonstrable in demo mode, say so and abort.
Do not capture random unrelated screens.

### 2. Decide capture mode (screenshot vs video)

- **Screenshot** (default): single PNG of the final relevant state. Use for
  layout changes, new columns, style tweaks, form additions.
- **Video** (`--video`): animated WebP of an interaction sequence. Use for
  filtering, sorting, navigation, overlays, animations -- anything where
  the change is in the *transition*, not just the end state.

### 2. Write an ad-hoc VHS tape

Get the current short commit hash for the filename:
```sh
git rev-parse --short HEAD                          # normal mode
git -C "$worktree_dir" rev-parse --short HEAD       # PR mode
```

Create `.claude/captures/<short-sha>-<descriptive-name>.tape`.

**Standard preamble (no PR number):**

```tape
Require micasa

Output .claude/captures/<short-sha>-<descriptive-name>.webm

Set Shell bash
Set FontFamily "Hack Nerd Font"
Set FontSize 32
Set Width 2400
Set Height 1200
Set Padding 20
Set Theme "Dracula"
Set CursorBlink false
Set TypingSpeed 0

Env NO_COLOR ""
Env TERM "xterm-256color"
Env COLORTERM "truecolor"
Env COLORFGBG "15;0"
Env PS1 ""

Hide
Type "exec micasa demo"
Enter
Sleep 5s
```

**PR mode preamble** (uses the locally built binary, no `Require`):

```tape
Output .claude/captures/<short-sha>-<descriptive-name>.webm

Set Shell bash
Set FontFamily "Hack Nerd Font"
Set FontSize 32
Set Width 2400
Set Height 1200
Set Padding 20
Set Theme "Dracula"
Set CursorBlink false
Set TypingSpeed 0

Env NO_COLOR ""
Env TERM "xterm-256color"
Env COLORTERM "truecolor"
Env COLORFGBG "15;0"
Env PS1 ""

Hide
Type "exec <absolute-path-to-worktree>/micasa demo"
Enter
Sleep 5s
```

After the preamble, add keystrokes to navigate to the target state.
Reference existing tapes in `docs/tapes/` for navigation patterns:
- `Type "D"` + `Sleep 2s` -- dismiss/toggle dashboard
- `Type "f"` -- advance to next tab
- `Type "b"` -- go to previous tab
- `Type "j"` / `Type "k"` -- navigate rows
- `Type "l"` / `Type "h"` -- navigate columns
- `Type "s"` -- sort by current column
- `Enter` -- drilldown / open
- `Escape` -- close overlay / go back
- Tab / Shift+Tab -- toggle house profile

**For screenshots:** navigate to target state hidden, then:

```tape
Show
Sleep 1.5s
Hide
Ctrl+Q
Sleep 1s
```

**For videos:** `Show` before the interaction begins, capture the full
action sequence with appropriate sleeps between keystrokes (0.4-0.8s
between navigation keys, 1-2s pauses at interesting states):

```tape
Show
Sleep 1s
# ... interaction keystrokes with sleeps ...
Sleep 1.5s
Hide
Ctrl+Q
Sleep 1s
```

Keep tapes short -- 5-10 seconds visible for screenshots, 10-20 seconds
for videos.

### 3. Create the capture directory

```sh
mkdir -p .claude/captures
```

### 4. Record

Both modes use `capture-adhoc` (bundles vhs, fonts, ffmpeg). In PR mode
the tape drops `Require micasa` and uses the absolute path instead, so
capture-adhoc's bundled micasa is never invoked.

For screenshot (PNG):
```sh
nix run '.#capture-adhoc' -- .claude/captures/<name>.tape
```

For video (animated WebP):
```sh
nix run '.#capture-adhoc' -- --video .claude/captures/<name>.tape
```

Output path is printed to stdout.

If VHS fails, check:
- Tape syntax (compare against `docs/tapes/dashboard.tape`)
- That the binary exists and runs (normal: `which micasa`, PR: `<path>/micasa --help`)
- That `Sleep` after app launch is long enough (5s minimum)

### 5. Present to user and ask about upload

Do NOT clean up the PR worktree yet — the user may want additional
captures from the same PR.

Show the user:
- What the capture shows
- The file path
- Which PR it targets (if PR mode)

Then ask: **"Upload this to the PR via `gh image`?"**

Do NOT upload without explicit approval.

### 6. Upload if approved

```sh
gh image .claude/captures/<name>.png   # screenshot
gh image .claude/captures/<name>.webp  # video
```

Output is a markdown image reference. Include it in:
- A PR comment: `gh pr comment <N> --body "<markdown-ref>"`
- The PR body (if creating a PR next)

## Notes

- Captures are ephemeral -- `.claude/captures/` is gitignored
- To promote a capture to a permanent demo tape, copy the `.tape` to
  `docs/tapes/` and adjust its `Output` path
- VHS recording takes ~1-2 minutes; do not record unnecessarily
- Existing tapes in `docs/tapes/` are the authoritative reference for
  keystroke patterns
- PR mode worktrees are kept alive for additional captures; clean up
  manually with `git worktree remove <dir>` when done
