<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Proactive Insights

Issue: #691

## Problem

The LLM is reactive -- users ask, it answers. But it has access to the full
data model and could surface unprompted observations: aging appliances,
spending patterns, overdue inspections. This is the feature that differentiates
from a spreadsheet.

## Design

### UX

Insights appear as the **last section** in the dashboard overlay, below all
deterministic sections (incidents, overdue, upcoming, projects, expiring).
Urgent/actionable items stay on top; insights are supplementary analysis.

```
 Incidents (2)
 ...

 Insights                                          2m ago
 ─────────────────────────────────────────────────────────
  Water heater is 12y old (avg lifespan 10-15y)    Appl.
  4 HVAC calls this year, $3,200 total             Maint.
  Roof last inspected 3y ago, hail-prone area      Maint.
```

- **Opt-in**: new `llm.insights` config field (`bool`). Off by default. Section
  hidden entirely when disabled or LLM not configured.
- **On-demand**: generated when dashboard opens (if stale/absent), not on app
  start. No latency on launch, no wasted API calls.
- **Non-blocking**: dashboard renders immediately with deterministic sections;
  Insights section shows a spinner until the LLM responds.
- **Cached per session**: reused across dashboard open/close. Auto-invalidated
  after data mutations (`reloadAfterMutation` sets `insightsStale = true`).
  Manual refresh via `r` key.
- **Navigable**: Enter on an insight row jumps to the relevant tab/entity,
  same as existing dashboard nav. Right column shows target tab abbreviation.
- **Staleness indicator**: dim relative timestamp ("2m ago") next to section
  header.

### Architecture

- **Config**: `llm.insights` bool field on `LLM` config struct. Env var
  `MICASA_LLM_INSIGHTS`. Stored as `*bool` for tri-state (nil = default off).
- **Prompt**: new `BuildInsightsPrompt()` in `internal/llm/prompt.go`. Receives
  full data dump + schema + date + house profile, instructs LLM to output a
  JSON object `{"insights": [{text, tab, entity_id}, ...]}` via
  `WithJSONSchema`. Emphasizes cross-entity observations, max 5-7 items, no
  duplication of existing dashboard sections.
- **Streaming**: uses `ChatStream()` so insight items appear incrementally in
  the dashboard as each complete JSON object finishes streaming. Partial JSON is
  parsed on each chunk using bracket-balanced extraction.
- **State**: `insightsState` struct on `dashState`. Tracks results, loading
  flag, staleness, error, generated-at timestamp, cancel func.
- **Mutations**: `reloadAfterMutation()` sets `insightsStale = true`. Next
  dashboard open triggers refresh.
- **Refresh**: `r` key on dashboard re-fetches insights (cancels in-flight
  request if any).

### Data types

```go
// insightItem is one LLM-generated insight.
type insightItem struct {
    Text     string          `json:"text"`
    Tab      string          `json:"tab"`       // tab name for navigation
    EntityID uint            `json:"entity_id"` // minimum 1; references a specific entity
    Category insightCategory `json:"category"`  // "attention", "stale", or "pattern"
}

// insightsState tracks the async insights fetch.
type insightsState struct {
    items       []insightItem
    loading     bool
    stale       bool
    err         error
    generatedAt time.Time
    cancel      context.CancelFunc
}
```

### Key flow

1. User opens dashboard (`D` key)
2. `loadDashboard()` runs deterministic sections
3. If `llm.insights` is enabled and LLM is configured:
   a. If insights are cached and not stale, reuse them
   b. Otherwise, start async fetch: `tea.Cmd` that calls `ChatComplete()` with
      `BuildInsightsPrompt()`, returns `insightsResultMsg`
4. Dashboard renders immediately. Insights section shows spinner while loading.
5. `insightsResultMsg` arrives: parse JSON, store items, set `generatedAt`
6. Dashboard re-renders with insight rows
7. On mutation: `insightsStale = true`. Next dashboard open re-fetches.
8. `r` key: cancel in-flight, re-fetch.

### Non-goals

- Auto-run on app start
- Per-pipeline LLM overrides for insights
- Persisting insights across sessions
- Notification badge outside dashboard

## Implementation plan

1. Add `Insights *bool` field to `config.LLM` with env var
2. Add `insightsEnabled` field to `llmConfig` and `Options`
3. Add `insightsState` to `dashState`
4. Add `BuildInsightsPrompt()` to `internal/llm/prompt.go`
5. Add `insightsResultMsg` type and async fetch in dashboard.go
6. Wire into `reloadAfterMutation()` for staleness
7. Render insights section in `dashboardView()`
8. Add `r` key handler for refresh in `handleDashboardKeys()`
9. Add navigation (Enter jumps to entity)
10. Add spinner tick handling for insights
11. Tests: user-flow tests via keystroke
12. Config tests for new field
