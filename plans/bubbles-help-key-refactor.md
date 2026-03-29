<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Bubbles Help + Key Binding Refactor

Replace custom help rendering and string-based key dispatch with idiomatic
`bubbles/key` bindings and a two-pane help overlay.

## Motivation

The codebase defines ~70 key constants as raw strings in `model.go` and
dispatches on `.String()` comparisons across 24 sites (17 `switch`
statements + 7 inline `if` guards). A separate custom help renderer in
`view_help.go` manually constructs help sections from its own `binding`
struct. This means keybinding definitions, dispatch, and help documentation
are three disconnected systems that must be kept in sync manually.

The bubbles library provides `key.Binding` (structured keybindings with
built-in help text and enabled/disabled state). Adopting it consolidates
key definitions, dispatch, and help into a single source of truth.

## Scope

- Migrate all key constants to `key.Binding` values on an `AppKeyMap` struct
- Replace all `.String()` key dispatch with `key.Matches()`
- Replace the help overlay with a two-pane navigable layout powered by
  `key.Binding` data
- Replace the status bar hint system with `help.Model.ShortHelpView()`
  (trial — revertible if UX degrades)
- Preserve existing keycap styling

## Out of scope

- Dashboard viewport migration (separate work)
- Stopwatch/timer adoption (investigated, current `time.Since()` approach is
  better for extraction elapsed-time display)
- Changes to which keys do what (pure refactor, no behavioral changes)
- Overlay-local hint bars (ops tree, column finder, extraction, chat) — these
  stay as-is using `helpItem()`/`renderKeys()` helpers

## Design

### AppKeyMap struct

New file `internal/app/keybindings.go`. Groups bindings by context matching
the help sections. Every binding that appears in a `.String()` dispatch
gets a named field.

```go
type AppKeyMap struct {
    // Global (pre-overlay, handled in model_update.go)
    Quit   key.Binding // ctrl+q
    Cancel key.Binding // ctrl+c

    // Common keys (handleCommonKeys — both normal + edit modes)
    ColLeft      key.Binding // h, left
    ColRight     key.Binding // l, right
    ColStart     key.Binding // ^
    ColEnd       key.Binding // $
    Help         key.Binding // ?
    HouseToggle  key.Binding // tab
    MagToggle    key.Binding // ctrl+o (also used in chat)
    FgExtract    key.Binding // ctrl+b

    // Normal mode (handleNormalKeys)
    TabNext       key.Binding // f
    TabPrev       key.Binding // b
    TabFirst      key.Binding // B
    TabLast       key.Binding // F
    EnterEditMode key.Binding // i
    Enter         key.Binding // enter (drill/follow/preview)
    Dashboard     key.Binding // D
    Sort          key.Binding // s
    SortClear     key.Binding // S
    ToggleSettled key.Binding // t
    FilterPin     key.Binding // n
    FilterToggle  key.Binding // N
    FilterClear   key.Binding // ctrl+n
    FilterNegate  key.Binding // !
    ColHide       key.Binding // c
    ColShowAll    key.Binding // C
    ColFinder     key.Binding // /
    DocSearch     key.Binding // ctrl+f
    DocOpen       key.Binding // o
    ToggleUnits   key.Binding // U
    Chat          key.Binding // @
    Escape        key.Binding // esc (close detail / clear status)

    // Edit mode
    Add          key.Binding // a
    QuickAdd     key.Binding // A (documents only)
    EditCell     key.Binding // e
    EditFull     key.Binding // E
    Delete       key.Binding // d (toggles delete/restore)
    HardDelete   key.Binding // D
    ReExtract    key.Binding // r (documents only)
    ShowDeleted  key.Binding // x
    HouseEdit    key.Binding // p
    ExitEdit     key.Binding // esc

    // Forms (inline if-guards in model_update.go:updateForm)
    FormSave        key.Binding // ctrl+s
    FormCancel      key.Binding // esc (triggers discard confirm if dirty)
    FormNextField   key.Binding // tab (help-only; huh dispatches internally)
    FormPrevField   key.Binding // shift+tab (help-only; huh dispatches internally)
    FormEditor      key.Binding // ctrl+e
    FormHiddenFiles key.Binding // H (file picker)

    // Chat
    ChatSend      key.Binding // enter
    ChatToggleSQL key.Binding // ctrl+s
    ChatHistoryUp key.Binding // up, ctrl+p
    ChatHistoryDn key.Binding // down, ctrl+n
    ChatHide      key.Binding // esc

    // Chat completer
    CompleterUp      key.Binding // up, ctrl+p
    CompleterDown    key.Binding // down, ctrl+n
    CompleterConfirm key.Binding // enter
    CompleterCancel  key.Binding // esc
    // (ctrl+q quit is shared with global Quit binding)

    // Calendar
    CalLeft      key.Binding // h, left
    CalRight     key.Binding // l, right
    CalUp        key.Binding // k, up
    CalDown      key.Binding // j, down
    CalPrevMonth key.Binding // H
    CalNextMonth key.Binding // L
    CalPrevYear  key.Binding // [
    CalNextYear  key.Binding // ]
    CalToday     key.Binding // t
    CalConfirm   key.Binding // enter
    CalCancel    key.Binding // esc

    // Dashboard (overrides for dashboard context)
    DashUp          key.Binding // k, up
    DashDown        key.Binding // j, down
    DashNextSection key.Binding // J, shift+down
    DashPrevSection key.Binding // K, shift+up
    DashTop         key.Binding // g
    DashBottom      key.Binding // G
    DashToggle      key.Binding // e
    DashToggleAll   key.Binding // E
    DashJump        key.Binding // enter

    // Doc search
    DocSearchUp      key.Binding // up, ctrl+p, ctrl+k
    DocSearchDown    key.Binding // down, ctrl+n, ctrl+j
    DocSearchConfirm key.Binding // enter
    DocSearchCancel  key.Binding // esc

    // Column finder
    ColFinderUp        key.Binding // up, ctrl+p
    ColFinderDown      key.Binding // down, ctrl+n
    ColFinderConfirm   key.Binding // enter
    ColFinderCancel    key.Binding // esc
    ColFinderClear     key.Binding // ctrl+u
    ColFinderBackspace key.Binding // backspace

    // Ops tree
    OpsUp       key.Binding // k, up
    OpsDown     key.Binding // j, down
    OpsExpand   key.Binding // enter, l, right
    OpsCollapse key.Binding // h, left
    OpsTabNext  key.Binding // f
    OpsTabPrev  key.Binding // b
    OpsTop      key.Binding // g
    OpsBottom   key.Binding // G
    OpsClose    key.Binding // esc

    // Extraction pipeline
    ExtCancel     key.Binding // esc (cancel/close extraction)
    ExtInterrupt  key.Binding // ctrl+c (interrupt running step)
    ExtUp         key.Binding // k, up (step cursor / viewport scroll)
    ExtDown       key.Binding // j, down
    ExtToggle     key.Binding // enter (expand/collapse step)
    ExtRemodel    key.Binding // r (pick different model, when done)
    ExtToggleTSV  key.Binding // t (toggle TSV display, when done)
    ExtAccept     key.Binding // a (accept results, when done)
    ExtExplore    key.Binding // x (enter table explore mode, when done)
    ExtBackground key.Binding // ctrl+b (background extraction)

    // Extraction explore mode (table preview navigation)
    ExploreUp       key.Binding // k, up
    ExploreDown     key.Binding // j, down
    ExploreLeft     key.Binding // h, left
    ExploreRight    key.Binding // l, right
    ExploreColStart key.Binding // ^
    ExploreColEnd   key.Binding // $
    ExploreTop      key.Binding // g
    ExploreBottom   key.Binding // G
    ExploreTabNext  key.Binding // f
    ExploreTabPrev  key.Binding // b
    ExploreAccept   key.Binding // a (accept results)
    ExploreExit     key.Binding // esc, x

    // Extraction model picker
    ExtModelUp        key.Binding // up, ctrl+p
    ExtModelDown      key.Binding // down, ctrl+n
    ExtModelConfirm   key.Binding // enter
    ExtModelCancel    key.Binding // esc
    ExtModelBackspace key.Binding // backspace

    // Help overlay (two-pane navigation)
    HelpSectionUp   key.Binding // k, up
    HelpSectionDown key.Binding // j, down
    HelpClose       key.Binding // esc, ?

    // Confirmations (discard, hard delete)
    ConfirmYes key.Binding // y
    ConfirmNo  key.Binding // n, esc

    // Inline input
    InlineConfirm key.Binding // enter
    InlineCancel  key.Binding // esc
}
```

Each binding carries keys and help text:

```go
func newAppKeyMap() AppKeyMap {
    return AppKeyMap{
        Quit: key.NewBinding(
            key.WithKeys(keyCtrlQ),
            key.WithHelp("ctrl+q", "quit"),
        ),
        DashDown: key.NewBinding(
            key.WithKeys(keyJ, keyDown),
            key.WithHelp(keyJ+"/"+keyK+"/"+symUp+"/"+symDown, "nav"),
        ),
        // ...
    }
}
```

The existing string constants (`keyCtrlQ`, `keyJ`, etc.) remain as private
helpers for `WithKeys()` calls. They stop being the dispatch mechanism.

**Multi-key bindings**: Cases like `case keyJ, keyDown:` become a single
binding with `WithKeys(keyJ, keyDown)`. The `WithHelp` display string uses
Unicode symbols for readability (e.g. `"j/k/↑/↓"`) — the display text is
independent of the actual key strings.

**Shared keys across contexts**: Some keys (j/k, esc, enter) mean different
things in different contexts (dashboard, calendar, chat, ops tree, help).
These get separate binding fields per context (e.g. `DashDown` vs
`CalDown` vs `HelpSectionDown` vs `ExtDown`). The dispatch site determines which
binding to match against based on the active overlay/mode, same as today.

**Cross-context bindings**: Some bindings are referenced from multiple
dispatch handlers. `MagToggle` (ctrl+o) is used in `handleCommonKeys`
and `handleChatKey`. `DocOpen` (o) is used in both `handleNormalKeys`
and `handleEditKeys`. A single binding field is shared — both dispatch
sites reference the same `m.keys.Xxx`. Comments on these fields note
the dual context.

**Table-delegated bindings**: Row navigation (`j`/`k`/`up`/`down`),
goto-top/bottom (`g`/`G`), half-page (`d`/`u`), and page-up/down are
all handled by the bubbles `table.KeyMap`, not by app-level dispatch.
Unhandled key messages fall through to `tab.Table.Update(msg)` at
`model_update.go:265`. In edit mode, `editTableKeyMap()` strips `d`
from `HalfPageDown` (freeing it for Delete) and leaves only `ctrl+d`.
This mode-swapping logic (`setAllTableKeyMaps`) stays as-is — these
bindings are **not** in `AppKeyMap`. The help overlay reads their
display text from `table.KeyMap` fields for the navigation section.

**Help-display-only entries**: Some items in the current help content
have no corresponding binding (e.g. "1-9 Jump to Nth option" in Forms).
These are handled by `msg.Text` checks, not `.String()` dispatch.
The `helpFormBindings()` method can include ad-hoc display-only bindings
created with `WithHelp` but no `WithKeys`, so they appear in help but
never match in dispatch.

**Text input fallback**: Several dispatch sites (column finder,
extraction model picker) have a `default` branch that handles arbitrary
printable text input via `key.Text` or `msg.String()`. These cannot become
`key.Matches()` calls — the `default`/text-input fallback stays as-is.
Only the named `case` branches above them migrate to `key.Matches()`.

### Enabling/disabling bindings by mode

The `help.KeyMap` interface requires `ShortHelp()` and `FullHelp()` methods
on the keymap. Since these methods need to reflect the current app mode,
**`Model` implements `help.KeyMap`**, not `AppKeyMap`:

```go
func (m *Model) ShortHelp() []key.Binding { ... }
func (m *Model) FullHelp() [][]key.Binding { ... }
```

`Model` reads `m.mode` and returns only the bindings relevant to the
current mode. For the full help overlay (which shows all sections), each
section is a `[]key.Binding` slice returned by a named method:

```go
func (m *Model) helpGlobalBindings() []key.Binding { ... }
func (m *Model) helpNavBindings() []key.Binding { ... }
func (m *Model) helpEditBindings() []key.Binding { ... }
func (m *Model) helpFormBindings() []key.Binding { ... }
func (m *Model) helpChatBindings() []key.Binding { ... }
```

These methods can conditionally include or exclude bindings based on state
(e.g. `DocSearch` only when on the documents tab, `Chat` only when LLM
client is configured).

### Dispatch migration

All 24 `.String()` dispatch sites change to `key.Matches()`:

Before:
```go
switch key.String() {
case keyCtrlQ:
    return m, tea.Quit
case keyJ, keyDown:
    m.moveDown()
}
```

After:
```go
switch {
case key.Matches(msg, m.keys.Quit):
    return m, tea.Quit
case key.Matches(msg, m.keys.DashDown):
    m.dashDown()
}
```

**All 24 dispatch sites** (verified by exhaustive grep for `.String()`
on key messages):

`switch` blocks (17):
1. `model_keys.go:handleDashboardKeys` — dashboard navigation
2. `model_keys.go:handleCommonKeys` — shared normal+edit keys
3. `model_keys.go:handleNormalKeys` — normal mode
4. `model_keys.go:handleEditKeys` — edit mode
5. `model_keys.go:handleCalendarKey` — date picker (includes bare `"t"`)
6. `chat.go:handleChatKey` — completer navigation
7. `chat.go` line 1192 — main chat input
8. `model_status.go:handleConfirmDiscard` — discard prompt (y/n/esc)
9. `model_status.go:handleConfirmHardDelete` — hard delete prompt (y/n/esc)
10. `search.go:handleDocSearchKey` — document search
11. `column_finder.go:handleColumnFinderKey` — column finder
12. `ops_tree.go:handleOpsTreeKey` — operations tree
13. `model.go:handleInlineInputKey` — inline input (esc/enter)
14. `model.go:helpOverlayKey` — help viewport (esc/?, g/G, viewport delegate)
15. `extraction.go:handleExtractionPipelineKey` — extraction steps
    (esc, ctrl+c, j/k, enter, r, t, a, x, ctrl+b)
16. `extraction.go:handleExtractionModelPickerKey` — extraction model
    picker (esc, up/down, enter, backspace, text input)
17. `extraction.go:handleExtractionExploreKey` — extraction table explore
    (esc, j/k, h/l, ^/$, g/G, b/f, a, x)

Inline `if` guards (7):
18. `model_update.go:28` — `typed.String() == keyCtrlQ` (global quit)
19. `model_update.go:44` — `typed.String() == keyCtrlC` (global cancel)
20. `model_update.go:278` — form ctrl+s save
21. `model_update.go:281` — form ctrl+e launch editor
22. `model_update.go:294` — form shift+h toggle hidden files
23. `model_update.go:325` — form esc discard-confirm gate
24. `model_keys.go:16` — dashboard `if key.String() != keyEnter` guard

### Help overlay: two-pane layout

Replace the current single-pane scrollable help with a two-pane navigable
overlay. This is a custom renderer powered by `key.Binding` data — we do
**not** use `help.FullHelpView()` because it renders columns without section
headers, which doesn't match the UX we want.

```
┌───────────────────────────────────────────────┐
│  Keyboard Shortcuts                           │
│                                               │
│    Global      │  J · K · ↑ · ↓  Rows        │
│  ▸ Nav Mode    │  H · L · ← · →  Columns     │
│    Edit Mode   │  ^ · $  First/last column    │
│    Forms       │  G · G  First/last row       │
│    Chat        │  D · U  Half page dn/up      │
│                │  B · F  Switch tabs          │
│                │  S · S  Sort / clear sorts ▼ │
│                                               │
│  J · K nav   ESC close                        │
└───────────────────────────────────────────────┘
```

**Left pane**: Static list of section names. j/k moves the cursor (▸).
Selecting a section populates the right pane. Uses dedicated
`HelpSectionUp`/`HelpSectionDown` bindings so help overlay dispatch is
self-contained.

**Right pane**: `viewport.Model` showing bindings for the selected section.
Scrolls independently when the section has more bindings than fit. Scroll
indicators (▲/▼) shown when content overflows. The viewport handles
pgup/pgdn and delegates unrecognized keys.

**Overlay sizing**: Fixed proportion of the terminal — e.g. 60% width,
70% height, centered. Minimum size clamped so the two panes remain usable.

**Data flow**: Each section's bindings come from the `helpXxxBindings()`
methods on `Model`. The renderer iterates the `[]key.Binding` slice and
formats each entry using the existing `renderKeysLight()` + `HeaderHint()`
styling. This preserves the current keycap look while reading from
`key.Binding` data.

**State**: New `helpState` struct replaces the current `*viewport.Model`
pointer:

```go
type helpState struct {
    section  int             // selected section index (left pane cursor)
    viewport viewport.Model  // right pane scroll state
}
```

**Navigation**:
- `j`/`k`: Move section cursor in the left pane. Right pane content
  updates and scroll resets to top.
- `g`/`G`/`pgup`/`pgdn`: Delegated to the right-pane viewport for
  scrolling within the selected section.
- `esc`/`?`: Close overlay.
- All other keys: delegated to the right-pane viewport (same pattern
  as the current `helpOverlayKey` default branch).

### Status bar hints

The current system uses two functions: `normalModeStatusHints()` and
`editModeStatusHelp()`, each constructing `statusHint` structs with
priority levels (0-3), compact/full variants, required anchors, and a
multi-phase fitting algorithm (`renderStatusHints()`).

`help.ShortHelpView()` provides width-aware truncation with ellipsis but
no priority system. It truncates left-to-right: bindings returned first by
`ShortHelp()` survive; later ones get dropped.

**Approach**: `Model.ShortHelp()` returns bindings ordered by priority
(anchors first, then descending importance). This maps priority levels to
position:

```go
func (m *Model) ShortHelp() []key.Binding {
    bindings := []key.Binding{m.keys.Help}  // always visible
    if m.mode == modeEdit {
        bindings = append(bindings, m.keys.Add, m.keys.EditCell, ...)
    } else {
        if hint := m.enterBinding(); hint.Enabled() {
            bindings = append(bindings, hint)
        }
        bindings = append(bindings, m.keys.EnterEditMode, ...)
    }
    if m.inDetail() {
        bindings = append(bindings, m.keys.Escape)
    }
    return bindings
}
```

**Lost capabilities vs current system**:
- No compact variants (current system falls back from "del/restore" to
  "del"). `ShortHelpView()` only has one text per binding. Acceptable
  loss — the full text will just truncate earlier.
- No mode badge. The mode badge ("NAV"/"EDIT") is not a keybinding. Prepend
  it before the help output: `modeBadge + " " + m.help.ShortHelpView(...)`.
- No per-hint zone marks for clickability. Zone marks can wrap the entire
  help output, or we add zone marks in a post-processing step.

**Overlay-local hint bars** (ops tree, column finder, chat, extraction,
forms, inline input) continue using `helpItem()`/`renderKeys()` helpers.
These are contextual, not part of the global help system. The helpers
survive the refactor.

## Phases

### Phase 1: key.Binding migration

- Create `keybindings.go` with `AppKeyMap` struct and `newAppKeyMap()`
- Add `keys AppKeyMap` field to `Model`
- Migrate all 24 dispatch sites from `.String()` to `key.Matches()`
  (17 switches + 7 inline guards)
- Fix bare `"t"` literal in `handleCalendarKey` — add `CalToday` binding
- Keep the custom help renderer temporarily, wired to read from `AppKeyMap`
  binding help text instead of manual `binding` structs
- Verify: all existing key dispatch tests pass, help overlay still works

### Phase 2: two-pane help overlay

- Add `helpState` struct with section cursor + viewport
- Add `HelpSectionUp`/`HelpSectionDown`/`HelpClose` bindings
- Build two-pane renderer reading from `helpXxxBindings()` methods
- Implement left-pane j/k navigation, right-pane viewport scrolling
- Overlay sized as fixed proportion of terminal, centered
- Style bindings using existing `renderKeysLight()` + `HeaderHint()`
- Remove old `helpContent()` and single-viewport approach
- Verify: help overlay navigable, sections display correctly, scroll works

### Phase 3: status bar hints (trivially revertible)

- Add `help.Model` to `Model` struct (used only for `ShortHelpView()`)
- Configure `help.Styles` to match keycap styling
- Implement `ShortHelp()` on `Model` with priority-ordered bindings
- Replace `normalModeStatusHints()` and `editModeStatusHelp()` with
  `help.ShortHelpView()` calls, prepending the mode badge
- Handle context-dependent hints via `SetHelp()` on bindings
- Address zone marks for clickability (wrap or post-process)
- Verify: status bar adapts to terminal width, mode changes reflected
- If UX degrades, revert this commit only — phases 1-2 are independent

## Testing strategy

Each phase verifies existing tests pass before and after changes. Phase 1
is the highest risk (touches dispatch) so test coverage of key handling is
verified first. New tests are added only where existing coverage is
insufficient. Phase 2 adds tests for the two-pane help navigation (section
switching, right-pane scroll).

## Files touched

- **New**: `internal/app/keybindings.go`
- **Heavy edits**: `internal/app/model.go` (key constants stay, add
  AppKeyMap field, helpState replaces helpViewport, helpOverlayKey
  rewritten; 2 switches), `internal/app/model_keys.go` (5 switches +
  1 inline guard),
  `internal/app/model_update.go` (2 global guards + 4 form guards),
  `internal/app/view_help.go` (two-pane renderer),
  `internal/app/view.go` (status bar hints)
- **Light edits**: `chat.go` (2 switches), `column_finder.go`,
  `model_status.go` (2 switches), `ops_tree.go`, `search.go`,
  `extraction.go` (3 switches: pipeline, model picker, explore)
