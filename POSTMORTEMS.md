<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Postmortems

Real examples of agent failure patterns in this repo. Read these before
attempting multi-iteration fixes.

---

## Cancellation bug: 14 fix commits for a 2-line root cause

**Feature**: ctrl+c cancellation of streaming LLM responses in the chat overlay.

**Symptom**: Pressing ctrl+c during SQL streaming produced a spurious "LLM
returned empty SQL" error, left a frozen spinner, and failed to show an
"Interrupted" indicator.

**What went wrong**: The agent (Claude 4.5 Sonnet) spent 14 commits adding
increasingly elaborate workarounds:

1. Added a `Cancelled` flag to chatState, checked it in multiple handlers.
2. Added special-case error suppression when the flag was set.
3. Added a second cancellation handler that duplicated the first.
4. Added flag-clearing logic in multiple places, creating ordering bugs.
5. Kept patching symptoms as each new flag interaction broke something else.

Each "fix" passed the specific scenario the agent was looking at but broke
another path. The test assertions checked internal state mutations rather
than observable behavior, so they kept passing even when the UI was broken.

**Root cause**: `waitForSQLChunk` and `waitForChunk` synthesized a
`Done: true` message when the stream channel closed (due to cancellation).
This fake "done" message had empty content, which downstream handlers
treated as a real completion with no SQL -- triggering the error.

**Actual fix**: Return `nil` (no Bubble Tea message) when the channel
closes instead of synthesizing a Done message. Two lines changed, zero
flags added. The message loop simply stops, and the cancellation handler
cleans up the UI state.

**Lessons**:

- When a fix doesn't work on the second try, the mental model of the bug
  is wrong. Stop patching and re-read the full code path.
- Flags and special cases are a smell. If you need a `Cancelled` bool to
  suppress errors, the errors shouldn't be generated in the first place.
- Test observable output (rendered UI), not internal state. The spinner
  was visibly frozen but all state-based tests passed.
- Concurrency bugs in message-passing systems are almost always about
  *what messages get sent*, not about *what flags are set when they arrive*.

---

## Postal code autofill: 1 hour for a 20-minute feature

**Feature**: Auto-fill city/state from postal code in the house form (#793).

**What went wrong**: Three compounding failures turned a straightforward
feature into 16 commits and over an hour of debugging.

### Failure 1: Mocks matched the code, not the API

During design, the real zippopotam.us API was fetched and showed JSON keys
with spaces (`"place name"`, `"state abbreviation"`). The struct tags were
written from memory with underscores (`"place_name"`, `"state_abbreviation"`).
Every test mock used the same wrong keys. All tests passed. The real API
silently returned zero values.

### Failure 2: Assumed huh's tab key synchronously moves focus

Built three blur detection approaches:
1. Compare focused field before/after a single `form.Update` call -- failed
   because huh returns a `NextField` *command* that executes on the next frame.
2. Track `lastFocusedField` across frames -- failed because
   `GetFocusedField()` returns unstable pointers across `form.Update` calls.
3. Intercept `nextFieldMsg` directly -- failed because the message arrives
   after the user's keystrokes, which go into the wrong field.

The fix was trivial: trigger on postal code *value change*, not field blur.
Every `updateForm` call checks if `values.PostalCode` changed and has >= 3
characters. No focus tracking needed.

### Failure 3: Didn't understand huh's buffer model

Setting `values.City = "Beverly Hills"` on the Go struct doesn't update
huh's internal `textinput` buffer. The form renders the buffer, not the
struct pointer. The city field appeared empty to the user even though the
struct was correct. Fix: call `cityInput.Value(&values.City)` to resync
huh's buffer from the pointer.

### Root cause

All three failures share the same root: **implementing from assumptions
instead of verifying against the real system.** The mocks validated
assumptions, not behavior.

### What would have prevented this

- Run a live API smoke test before writing mocks (catches #1 in 30 seconds)
- Read huh's `Input.Update` and `Group.Update` source before designing
  blur detection (catches #2)
- Read huh's `Input.View` to understand buffer vs accessor rendering
  (catches #3)

### Rules added

- **Never mock from memory**: Copy real API responses into test mocks.
  Always include a live API smoke test for external integrations.
- **Read framework source before designing integration**: Don't assume
  sync/async behavior -- verify it in the source.
