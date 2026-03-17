<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# TUI Sync Integration Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development
> (if subagents available) or superpowers:executing-plans to implement this plan.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add background sync to the TUI so Pro subscribers get automatic
pull/push without running `micasa pro sync` manually.

**Architecture:** Sync runs as `tea.Cmd` closures alongside the Bubble Tea
program. On startup, after every mutation (debounced 2s), and periodically
(60s), a `doSync` command pulls then pushes via a `sync.Engine`. The engine
accepts a `context.Context` for cancellation on quit. Communication uses
typed `tea.Msg` values through the program's event loop. The status bar
shows a sync indicator glyph. SQLite WAL mode handles concurrent
reader/writer access between the UI goroutine and the sync command goroutine.

**Tech Stack:** Bubble Tea v2 (`tea.Cmd`, `tea.Msg`), existing `sync.Client`,
`data.Store`, `crypto` package, `lipgloss.AdaptiveColor`.

**Thread safety:** `data.Store` wraps a GORM/SQLite connection configured with
WAL mode and `_busy_timeout=5000`. The Bubble Tea main goroutine reads the
store (tab reloads, etc.) while `tea.Cmd` goroutines write (sync). SQLite
WAL supports concurrent readers with one writer; the busy timeout handles
brief writer contention. No additional locking is needed.

**Debounce strategy (resolved):** Use a 2s debounce after local mutations,
independent of the 60s periodic tick. The periodic tick catches remote
changes; the debounce pushes local changes quickly. The debounce is
implemented via `tea.Tick(2s)` — each mutation resets it by storing a
generation counter; the tick callback checks the counter matches before
triggering sync.

---

## File Structure

### Create

- **`internal/sync/engine.go`** — `Engine` struct, `SyncResult`,
  `NewEngine()`, `Sync(ctx)` method. Extracted from `cmd/micasa/pro.go`.
- **`internal/sync/engine_test.go`** — Engine round-trip tests
- **`internal/app/sync.go`** — message types (`syncStartedMsg`,
  `syncDoneMsg`, `syncErrorMsg`), `syncStatus` enum, `doSync` command
  constructor, `syncTick`, `syncIndicator` renderer, `syncConfig` struct
- **`internal/app/sync_test.go`** — state transition tests, integration test

### Modify

- **`internal/app/types.go`** — add sync config setter method on `Options`
- **`internal/app/model.go`** — add sync fields to `Model`, wire sync
  messages in `Update`, call debounced sync after mutations, start sync in
  `Init`, cancel sync on quit
- **`internal/app/view.go`** — render sync indicator in status bar
- **`internal/app/styles.go`** — add sync indicator styles
- **`cmd/micasa/pro.go`** — delegate to `sync.Engine`, export
  `tryLoadSyncConfig` helper
- **`cmd/micasa/main.go`** — detect Pro setup, pass sync config to `Options`

---

## Task 1: Extract sync engine from pro.go

This is first because both the TUI and CLI need it.

**Files:**
- Create: `internal/sync/engine.go`
- Create: `internal/sync/engine_test.go`
- Modify: `cmd/micasa/pro.go`

- [ ] **Step 1: Write failing test for Engine.Sync**

Test that `Engine.Sync(ctx)` performs pull then push and returns a
`SyncResult` with pulled/pushed counts. Use a `MemStore` relay handler
as the test server (`httptest.NewServer`).

- [ ] **Step 2: Create Engine struct**

```go
// internal/sync/engine.go

package sync

import "context"

type Engine struct {
    store       *data.Store
    client      *Client
    householdID string
}

type SyncResult struct {
    Pulled    int
    Pushed    int
    Conflicts int
    BlobsUp   int
    BlobsDown int
    BlobErrs  int
}

func NewEngine(store *data.Store, client *Client, householdID string) *Engine {
    return &Engine{store: store, client: client, householdID: householdID}
}

// Sync performs a full pull+push cycle. The context is checked between
// phases so the caller can cancel mid-sync (e.g., on app shutdown).
func (e *Engine) Sync(ctx context.Context) (SyncResult, error) {
    // ... pull, push, blob upload, blob fetch
}
```

Move `pullAll`, `pushAll`, `uploadPendingBlobs`, `fetchPendingBlobs` from
`cmd/micasa/pro.go` into private methods on `Engine`. Each method should
check `ctx.Err()` before starting network calls.

- [ ] **Step 3: Run engine test**

Run: `go test -shuffle=on -run TestEngine ./internal/sync/`
Expected: PASS

- [ ] **Step 4: Update pro.go to use Engine**

Replace `pullAll`/`pushAll`/blob helpers with:
```go
engine := sync.NewEngine(deps.store, client, deps.device.HouseholdID)
result, err := engine.Sync(context.Background())
```

- [ ] **Step 5: Run all tests**

Run: `go test -shuffle=on ./internal/sync/ ./cmd/micasa/`
Expected: all pass

- [ ] **Step 6: Commit**

```
feat(sync): extract Engine from pro.go for reuse by TUI
```

---

## Task 2: Sync message types, state enum, and commands

**Files:**
- Create: `internal/app/sync.go`

- [ ] **Step 1: Create sync.go with types and commands**

```go
package app

import (
    "context"
    "time"

    "github.com/cpcloud/micasa/internal/crypto"
    "github.com/cpcloud/micasa/internal/sync"
    tea "charm.land/bubbletea/v2"
)

type syncStatus int

const (
    syncIdle     syncStatus = iota // not configured
    syncSynced                     // last sync ok
    syncSyncing                    // in progress
    syncOffline                    // last sync failed
    syncConflict                   // ok but conflicts detected
)

// syncConfig holds resolved Pro credentials. Unexported — main.go uses
// the Options.SetSync() setter to pass it in.
type syncConfig struct {
    relayURL    string
    token       string
    householdID string
    key         crypto.HouseholdKey
}

// --- tea.Msg types ---

type syncStartedMsg struct{}
type syncDoneMsg struct {
    Pulled    int
    Pushed    int
    Conflicts int
}
type syncErrorMsg struct{ Err error }
type syncTickMsg time.Time

// syncDebounceMsg fires 2s after a local mutation.
// gen is the generation counter at dispatch time; if it doesn't match
// the model's current counter, a newer mutation superseded this one.
type syncDebounceMsg struct{ gen int }

// --- tea.Cmd constructors ---

func doSync(engine *sync.Engine, ctx context.Context) tea.Cmd {
    return tea.Batch(
        func() tea.Msg { return syncStartedMsg{} },
        func() tea.Msg {
            result, err := engine.Sync(ctx)
            if err != nil {
                return syncErrorMsg{Err: err}
            }
            return syncDoneMsg{
                Pulled:    result.Pulled,
                Pushed:    result.Pushed,
                Conflicts: result.Conflicts,
            }
        },
    )
}

func syncTick() tea.Cmd {
    return tea.Tick(60*time.Second, func(t time.Time) tea.Msg {
        return syncTickMsg(t)
    })
}

func syncDebounce(gen int) tea.Cmd {
    return tea.Tick(2*time.Second, func(_ time.Time) tea.Msg {
        return syncDebounceMsg{gen: gen}
    })
}

// syncIndicator renders the status bar glyph.
func (m *Model) syncIndicator() string {
    if m.syncCfg == nil {
        return ""
    }
    switch m.syncStatus {
    case syncSynced:
        return appStyles.SyncSynced().Render("◈")
    case syncSyncing:
        return appStyles.SyncSyncing().Render("◉")
    case syncOffline:
        return appStyles.SyncOffline().Render("○")
    case syncConflict:
        return appStyles.SyncConflict().Render("!")
    default:
        return ""
    }
}
```

Note: `syncConfig` does NOT store `lastSeq` — the engine reads it fresh
from `store.GetSyncDevice()` on each sync to avoid stale reads.

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/app/`

- [ ] **Step 3: Commit**

```
feat(app): add sync message types, commands, and indicator renderer
```

---

## Task 3: Sync indicator styles

**Files:**
- Modify: `internal/app/styles.go`

- [ ] **Step 1: Add sync indicator styles to Styles struct**

Add four private fields with public accessors:
- `syncSynced` — green/success color (Wong `#009E73`) for `◈`
- `syncSyncing` — muted color (Wong `#CC79A7`) for `◉`
- `syncOffline` — dim/gray color for `○`
- `syncConflict` — warning/orange color (Wong `#E69F00`) for `!`

Follow the existing `appStyles` singleton pattern with `AdaptiveColor`.

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/app/`

- [ ] **Step 3: Commit**

```
feat(app): add sync indicator styles to appStyles
```

---

## Task 4: Wire sync into Model

**Files:**
- Modify: `internal/app/types.go`
- Modify: `internal/app/model.go`
- Modify: `internal/app/sync.go`

- [ ] **Step 1: Add sync config setter to Options**

In `types.go`, add an unexported `syncCfg` field and a setter:
```go
type Options struct {
    // ... existing fields ...
    syncCfg *syncConfig
}

func (o *Options) SetSync(relayURL, token, householdID string, key crypto.HouseholdKey) {
    o.syncCfg = &syncConfig{
        relayURL:    relayURL,
        token:       token,
        householdID: householdID,
        key:         key,
    }
}
```

This keeps `syncConfig` unexported while allowing `cmd/micasa` to set it.

- [ ] **Step 2: Add sync fields to Model**

```go
// In Model struct:
syncStatus      syncStatus
syncCfg         *syncConfig
syncEngine      *sync.Engine       // nil when sync disabled
syncCtx         context.Context    // cancelled on quit
syncCancel      context.CancelFunc // cancels in-flight sync on quit
syncDebounceGen int                // generation counter for debounce
syncPendingReload bool             // true when pulled data awaits form close
```

In `NewModel`, if `options.syncCfg != nil`:
1. Create `sync.NewClient(cfg.relayURL, cfg.token, cfg.key)`
2. Create `sync.NewEngine(store, client, cfg.householdID)`
3. `m.syncCtx, m.syncCancel = context.WithCancel(context.Background())`
4. Set `m.syncStatus = syncSyncing` (first sync is about to start)

- [ ] **Step 3: Wire Init**

In `Model.Init()`, if `m.syncEngine != nil`:
```go
return tea.Batch(existingCmd, doSync(m.syncEngine, m.syncCtx), syncTick())
```

- [ ] **Step 4: Wire Update message handlers**

Add to the `switch msg := msg.(type)` in `Update`:

```go
case syncStartedMsg:
    m.syncStatus = syncSyncing
    return m, nil

case syncDoneMsg:
    if msg.Conflicts > 0 {
        m.syncStatus = syncConflict
    } else {
        m.syncStatus = syncSynced
    }
    if msg.Pulled > 0 {
        if m.mode == modeForm {
            // Defer reload until form closes to avoid clobbering edits.
            m.syncPendingReload = true
        } else {
            m.reloadAll()
        }
    }
    return m, nil

case syncErrorMsg:
    m.syncStatus = syncOffline
    return m, nil

case syncTickMsg:
    if m.syncEngine == nil || m.syncStatus == syncSyncing {
        return m, syncTick() // re-arm without starting sync
    }
    return m, tea.Batch(doSync(m.syncEngine, m.syncCtx), syncTick())

case syncDebounceMsg:
    if msg.gen != m.syncDebounceGen || m.syncEngine == nil || m.syncStatus == syncSyncing {
        return m, nil // stale or already syncing
    }
    return m, doSync(m.syncEngine, m.syncCtx)
```

- [ ] **Step 5: Wire mutation-triggered debounce**

In `reloadAfterMutation()`, after the existing reload logic, if sync is
configured:
```go
m.syncDebounceGen++
return syncDebounce(m.syncDebounceGen)
```

Note: `reloadAfterMutation` currently returns nothing (void). It needs to
return a `tea.Cmd` so the caller can batch it. Audit all call sites.

- [ ] **Step 6: Wire quit cleanup**

In the quit handler (where `cancelChatOperations()` and
`cancelAllExtractions()` are called), add:
```go
if m.syncCancel != nil {
    m.syncCancel()
}
```

This cancels any in-flight sync HTTP requests via the context.

- [ ] **Step 7: Handle deferred reload after form close**

When a form closes (mode transitions from `modeForm` to another mode), if
sync pulled data while the form was open, call `m.reloadAll()`. Track this
with a `syncPendingReload bool` field on Model.

- [ ] **Step 8: Verify it compiles**

Run: `go build ./internal/app/`

- [ ] **Step 9: Commit**

```
feat(app): wire background sync into Model lifecycle
```

---

## Task 5: Render sync indicator in status bar

**Files:**
- Modify: `internal/app/view.go`

- [ ] **Step 1: Add indicator to status bar**

In the status bar rendering (likely `withStatusMessage` or equivalent),
prepend `m.syncIndicator()` before the status text. Zone-mark it for
clickability:
```go
m.zones.Mark("sync-indicator", m.syncIndicator())
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/app/`

- [ ] **Step 3: Commit**

```
feat(app): render sync indicator glyph in status bar
```

---

## Task 6: Wire startup detection in main.go

**Files:**
- Modify: `cmd/micasa/main.go`
- Create or modify: `cmd/micasa/sync_config.go`

- [ ] **Step 1: Create tryLoadSyncConfig helper**

```go
// cmd/micasa/sync_config.go

func tryLoadSyncConfig(store *data.Store, opts *app.Options) {
    dev, err := store.GetSyncDevice()
    if err != nil || dev.HouseholdID == "" {
        return // Pro not set up, silently skip
    }
    secretDir, err := crypto.SecretsDir()
    if err != nil {
        return
    }
    token, err := crypto.LoadDeviceToken(secretDir)
    if err != nil {
        return
    }
    key, err := crypto.LoadHouseholdKey(secretDir)
    if err != nil {
        return
    }
    opts.SetSync(dev.RelayURL, token, dev.HouseholdID, key)
}
```

- [ ] **Step 2: Call from main.go**

After building `Options` and before `app.NewModel()`:
```go
tryLoadSyncConfig(store, &opts)
```

- [ ] **Step 3: Verify it compiles**

Run: `go build ./cmd/micasa/`

- [ ] **Step 4: Commit**

```
feat(app): detect Pro setup at startup and enable background sync
```

---

## Task 7: Tests for sync state transitions

**Files:**
- Create: `internal/app/sync_test.go`

- [ ] **Step 1: Write state transition tests**

Using `sendMsg` (or similar test helper) to inject messages into a Model:
- `syncStartedMsg` → `m.syncStatus == syncSyncing`
- `syncDoneMsg{Pulled: 5}` → `m.syncStatus == syncSynced`
- `syncDoneMsg{Conflicts: 1}` → `m.syncStatus == syncConflict`
- `syncErrorMsg{Err: ...}` → `m.syncStatus == syncOffline`
- `syncTickMsg` when `syncStatus == syncSyncing` → no cmd returned (guard)
- `syncDebounceMsg` with stale gen → no cmd returned

These follow the project convention of driving through the Model's Update
method with real message types.

- [ ] **Step 2: Write indicator rendering test**

Assert the correct glyph string for each syncStatus value.

- [ ] **Step 3: Run tests**

Run: `go test -shuffle=on ./internal/app/`

- [ ] **Step 4: Commit**

```
test(app): sync state transitions and indicator rendering
```

---

## Task 8: Integration test with mock relay

**Files:**
- Modify: `internal/app/sync_test.go`

- [ ] **Step 1: Write end-to-end sync test**

Drive sync through a user-visible mutation (form save), not by injecting
sync messages directly:

1. Stand up an `httptest.NewServer` with a `relay.NewHandler(relay.NewMemStore())`
2. Create a household + device on the test relay
3. Create a Model with `SetSync(...)` pointing to the test server
4. Call `Init`, let the startup sync complete (process messages until
   `syncDoneMsg` arrives — startup sync pushes 0 ops, pulls 0)
5. Simulate a user mutation: `openAddForm(m)`, fill fields, `sendKey(m, "ctrl+s")`
   — this creates a local oplog entry and triggers `syncDebounce`
6. Advance time past the 2s debounce: process `syncDebounceMsg`
7. Process resulting `syncStartedMsg` and `syncDoneMsg`
8. Assert: ops were pushed to the test relay (query relay MemStore)
9. Assert: `m.syncStatus == syncSynced`
10. Assert: `m.syncIndicator()` contains `◈`

This tests the full user flow: keypress → mutation → debounce → sync → indicator.

- [ ] **Step 2: Run tests**

Run: `go test -shuffle=on ./internal/app/`

- [ ] **Step 3: Commit**

```
test(app): integration test for TUI sync with mock relay
```

---

## Verification

After all tasks:

```bash
go build ./...
go test -shuffle=on ./...
```

All packages must compile and all tests must pass.

## Design Decisions

1. **2s debounce, not accumulate-to-tick**: Local mutations trigger sync
   after 2s of quiet. The 60s tick independently catches remote changes.
   This gives responsive local push without excessive relay traffic.

2. **Skip reload during form edit**: If user is in `modeForm` when remote
   ops arrive, set `syncPendingReload` and defer `reloadAll()` until the
   form closes. Prevents clobbering unsaved edits.

3. **Engine reads lastSeq from DB**: Not from startup config. This avoids
   stale reads if multiple sync cycles run before the config is refreshed.

4. **Context cancellation on quit**: `Engine.Sync(ctx)` checks
   `ctx.Err()` between phases (pull → push → blob up → blob down). On app
   quit, the context is cancelled, causing in-flight HTTP requests to abort
   promptly via the `http.Client` context.

5. **Zone-marked indicator**: The sync glyph is zone-marked for
   clickability per CLAUDE.md conventions, enabling future click-to-sync.
