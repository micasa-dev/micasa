<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Fix: ANSI escape codes leak and spinner hangs during VHS recording

Issue: #636

## Problem

When recording the LLM chat overlay with VHS, two rendering issues appear
that don't occur in the live app:

1. ANSI escape codes show up as visible text in the input area after
   streaming finishes
2. The spinner hangs instead of stopping when the response completes

The live app works fine because cursor blink and other events trigger
continuous redraws that mask stale viewport state. VHS disables cursor
blink and has no mouse/resize events during Sleep, so stale frames persist.

## Root cause

### OSC 11 background color query leaks into stdin

`glamour.WithAutoStyle()` calls `termenv.HasDarkBackground()`, which sends
an OSC 11 query (`\033]11;?\033\\`) to the terminal to detect background
color. In VHS's virtual terminal (go-vte), the response arrives through
the pty and gets interpreted by bubbletea as user input, ending up as
literal text in the focused textinput.

The fix caches the glamour style at process init time (before bubbletea
takes over stdin) using `lipgloss.HasDarkBackground()` and
`golang.org/x/term.IsTerminal()`, then passes the cached style to
`glamour.WithStyles()` instead of `glamour.WithAutoStyle()`.

### Contributing factors (also fixed)

- **Streaming completion order**: `refreshChatViewport()` ran while
  `Streaming` was still true, caching a stale viewport with spinner.
  In VHS, no subsequent events triggered a refresh.
- **ANSI in LLM content**: Some models emit ANSI codes in responses.
  These are now stripped with `ansi.Strip()` before storage.
- **Cursor blink cmd discarded**: `openChat()` called `ti.Focus()` but
  discarded the returned `tea.Cmd`, so no cursor blink timer started.
- **Non-key messages swallowed**: `dispatchOverlay` dropped non-key
  messages (like cursor blink ticks) when an overlay was active.

## Changes

1. **Cache glamour style at init** (`view.go`): detect dark/light once at
   startup; use `glamour.WithStyles()` instead of `glamour.WithAutoStyle()`
2. **Fix streaming completion order** (`chat.go`): set `Streaming = false`
   before `refreshChatViewport()` when Done (same for `StreamingSQL`)
3. **Strip ANSI from streamed content** (`chat.go`): `ansi.Strip()` on
   chunk content before appending to messages
4. **Return cursor blink cmd from `openChat`** (`chat.go`): capture
   `ti.Focus()` return so the cursor blinks in VHS
5. **Forward non-key messages to chat input** (`model.go`): let cursor
   blink ticks through `dispatchOverlay` when chat is focused
6. **Update VHS tape** (`docs/tapes/llm-chat.tape`): add follow-up
   question to demonstrate multi-turn conversation
